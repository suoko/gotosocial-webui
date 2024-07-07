package main

import (
    "context"
    "encoding/json"
    "fmt"
    "html/template"
    "log"
    "net/http"
    "sync"
    "time"

    "github.com/mattn/go-mastodon"
)

// Store client credentials globally with a mutex for thread safety
var (
    mu     sync.Mutex
    server string
    app    *mastodon.Application
)

// PageData holds the data to be rendered in the HTML template
type PageData struct {
    Timeline []*mastodon.Status
    Server   string
    Code     string
}

func main() {
    // Define the handler function for the root route
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        // Check if the access token cookie is present
        tokenCookie, err := r.Cookie("access_token")
        serverCookie, err := r.Cookie("server")
        if err != nil || tokenCookie == nil || serverCookie == nil {
            // If no token, ask the user to provide the Mastodon instance URL
            instance := r.FormValue("instance")
            if instance == "" {
                tmpl := template.Must(template.New("instance").Parse(`
                    <!DOCTYPE html>
                    <html>
                    <head>
                        <title>Gotosocial Connect</title>
                        <style>
                            body { font-family: Arial, sans-serif; margin: 20px; }
                            form { max-width: 300px; margin: 0 auto; }
                            label { display: block; margin-bottom: 8px; }
                            input { width: 100%; padding: 8px; margin-bottom: 16px; }
                            button { padding: 10px 15px; background-color: #4CAF50; color: white; border: none; cursor: pointer; }
                            button:hover { background-color: #45a049; }
                        </style>
                    </head>
                    <body>
                        <h2>Connect to Gotosocial</h2>
                        <form method="POST">
                            <label for="instance">Enter Gotosocial instance URL:</label>
                            <input type="text" id="instance" name="instance" required>
                            <button type="submit">Submit</button>
                        </form>
                    </body>
                    </html>
                `))
                tmpl.Execute(w, nil)
                return
            }

            // Register the application
            app, err = mastodon.RegisterApp(context.Background(), &mastodon.AppConfig{
                Server:       instance,
                ClientName:   "Gotosocial-webui",
                RedirectURIs: "http://localhost:8080/callback",
                Scopes:       "read write follow",
                Website:      instance,
            })
            if err != nil {
                http.Error(w, "Error registering app", http.StatusInternalServerError)
                log.Fatalf("Error registering app: %v", err)
                return
            }

            // Store the server URL
            mu.Lock()
            server = instance
            mu.Unlock()

            // Redirect to Mastodon authorization page
            authURL := app.AuthURI
            http.Redirect(w, r, authURL, http.StatusFound)
            return
        }

        // Fetch the home timeline using the access token
        accessToken := tokenCookie.Value
        server := serverCookie.Value
        client := mastodon.NewClient(&mastodon.Config{
            Server:      server,
            AccessToken: accessToken,
        })

        // Fetch the home timeline
        ctx := context.Background()
        timeline, err := client.GetTimelineHome(ctx, nil)
        if err != nil {
            http.Error(w, "Failed to fetch timeline", http.StatusInternalServerError)
            log.Println("Failed to fetch timeline:", err)
            return
        }

        // Define the HTML template for displaying the timeline
        tmpl := template.Must(template.New("timeline").Funcs(template.FuncMap{
            "safeHTML": func(text string) template.HTML {
                return template.HTML(text)
            },
        }).Parse(`
            <!DOCTYPE html>
            <html>
            <head>
                <title>Gotosocial Timeline</title>
                <style>
                    body { font-family: Arial, sans-serif; margin: 20px; }
                    ul { list-style-type: none; padding: 0; }
                    li { margin-bottom: 20px; padding: 10px; border: 1px solid #ddd; border-radius: 5px; position: relative; }
                    li:nth-child(odd) { background-color: #f9f9f9; }
                    .avatar { width: 50px; height: 50px; border-radius: 25px; vertical-align: middle; }
                    .username { font-weight: bold; margin-left: 10px; }
                    .icons { margin-top: 10px; }
                    .icon { margin-right: 10px; cursor: pointer; }
                    .reply-form { display: none; margin-top: 10px; }
                </style>
                <script>
                    function toggleReplyForm(postId) {
                        var replyForm = document.getElementById('reply-form-' + postId);
                        replyForm.style.display = 'block';
                    }
                </script>
            </head>
            <body>
                <h2>Gotosocial Home Timeline</h2>
                <ul>
                    {{range .Timeline}}
                        <li id="post-{{.ID}}">
                            <img src="{{.Account.Avatar}}" alt="avatar" class="avatar">
                            <span class="username">{{.Account.Username}}</span>
                            <p>{{safeHTML .Content}}</p>
                            <div class="icons">
                                <span class="icon" onclick="toggleReplyForm('{{.ID}}')">↩️</span>
                                <span class="icon" onclick="boost('{{.ID}}')">🔄</span>
                                <span class="icon" onclick="favourite('{{.ID}}')">⭐</span>
                            </div>
                            <div id="reply-form-{{.ID}}" class="reply-form">
                                <form onsubmit="submitReply(event, '{{.Account.Username}}', '{{.ID}}'); return false;">
                                    <textarea id="reply-text-{{.ID}}" rows="2" cols="30" placeholder="Reply to @{{.Account.Username}}"></textarea><br>
                                    <button type="submit">Send</button>
                                </form>
                            </div>
                        </li>
                    {{end}}
                </ul>
                <script>
                    function submitReply(event, username, postId) {
                        event.preventDefault();
                        var replyText = document.getElementById('reply-text-' + postId).value;
                        fetch('/reply', {
                            method: 'POST',
                            headers: {
                                'Content-Type': 'application/json',
                            },
                            body: JSON.stringify({ id: postId, replyText: '@' + username + ' ' + replyText })
                        }).then(response => {
                            if (response.ok) {
                                alert('Replied successfully');
                                document.getElementById('reply-text-' + postId).value = '';
                                document.getElementById('reply-form-' + postId).style.display = 'none';
                            } else {
                                alert('Failed to reply');
                            }
                        });
                    }

                    function boost(id) {
                        fetch('/boost', {
                            method: 'POST',
                            headers: {
                                'Content-Type': 'application/json',
                            },
                            body: JSON.stringify({ id: id })
                        }).then(response => {
                            if (response.ok) {
                                alert('Boosted successfully');
                            } else {
                                alert('Failed to boost');
                            }
                        });
                    }

                    function favourite(id) {
                        fetch('/favourite', {
                            method: 'POST',
                            headers: {
                                'Content-Type': 'application/json',
                            },
                            body: JSON.stringify({ id: id })
                        }).then(response => {
                            if (response.ok) {
                                alert('Favourited successfully');
                            } else {
                                alert('Failed to favourite');
                            }
                        });
                    }
                </script>
            </body>
            </html>
        `))

        // Render the template with the timeline data
        data := PageData{Timeline: timeline}
        if err := tmpl.Execute(w, data); err != nil {
            http.Error(w, "Failed to render template", http.StatusInternalServerError)
            log.Println("Failed to render template:", err)
        }
    })

    // Handle the callback from the authorization page
    http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
        code := r.URL.Query().Get("code")
        if code == "" {
            http.Error(w, "Authorization code not found", http.StatusBadRequest)
            return
        }

        // Retrieve the server URL
        mu.Lock()
        localServer := server
        localApp := app
        mu.Unlock()

        if localApp == nil {
            http.Error(w, "Application not registered", http.StatusInternalServerError)
            log.Println("Application not registered")
            return
        }

        // Create a client with the app credentials
        client := mastodon.NewClient(&mastodon.Config{
            Server:       localServer,
            ClientID:     localApp.ClientID,
            ClientSecret: localApp.ClientSecret,
        })

        // Authenticate and get the access token
        err := client.AuthenticateToken(context.Background(), code, "http://localhost:8080/callback")
        if err != nil {
            http.Error(w, "Failed to authenticate", http.StatusInternalServerError)
            log.Fatalf("Error authenticating: %v", err)
            return
        }

        // Save the access token and server URL in cookies
        http.SetCookie(w, &http.Cookie{
            Name:    "access_token",
            Value:   client.Config.AccessToken,
            Expires: time.Now().Add(24 * time.Hour),
            Path:    "/",
        })
        http.SetCookie(w, &http.Cookie{
            Name:    "server",
            Value:   localServer,
            Expires: time.Now().Add(24 * time.Hour),
            Path:    "/",
        })

        // Redirect to the home page
        http.Redirect(w, r, "/", http.StatusFound)
    })

    // Define handlers for reply, boost, and favourite actions
    http.HandleFunc("/reply", func(w http.ResponseWriter, r *http.Request) {
        var req struct {
            ID        string `json:"id"`
            ReplyText string `json:"replyText"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            http.Error(w, "Invalid request", http.StatusBadRequest)
            return
        }

        cookie, err := r.Cookie("access_token")
        if err != nil {
            http.Error(w, "Not authenticated", http.StatusUnauthorized)
            return
        }

        client := mastodon.NewClient(&mastodon.Config{
            Server:       server,
            ClientID:     app.ClientID,
            ClientSecret: app.ClientSecret,
            AccessToken:  cookie.Value,
        })

        ctx := context.Background()
        if _, err := client.PostStatus(ctx, &mastodon.Toot{
            InReplyToID: mastodon.ID(req.ID),
            Status:      req.ReplyText,
            Visibility:  mastodon.VisibilityPublic,
        }); err != nil {
            http.Error(w, "Failed to reply", http.StatusInternalServerError)
            return
        }

        w.WriteHeader(http.StatusOK)
    })

    http.HandleFunc("/boost", func(w http.ResponseWriter, r *http.Request) {
        var req struct {
            ID string `json:"id"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            http.Error(w, "Invalid request", http.StatusBadRequest)
            return
        }

        cookie, err := r.Cookie("access_token")
        if err != nil {
            http.Error(w, "Not authenticated", http.StatusUnauthorized)
            return
        }

        client := mastodon.NewClient(&mastodon.Config{
            Server:       server,
            ClientID:     app.ClientID,
            ClientSecret: app.ClientSecret,
            AccessToken:  cookie.Value,
        })

        ctx := context.Background()
        if _, err := client.Reblog(ctx, mastodon.ID(req.ID)); err != nil {
            http.Error(w, "Failed to boost", http.StatusInternalServerError)
            return
        }

        w.WriteHeader(http.StatusOK)
    })

    http.HandleFunc("/favourite", func(w http.ResponseWriter, r *http.Request) {
        var req struct {
            ID string `json:"id"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            http.Error(w, "Invalid request", http.StatusBadRequest)
            return
        }

        cookie, err := r.Cookie("access_token")
        if err != nil {
            http.Error(w, "Not authenticated", http.StatusUnauthorized)
            return
        }

        client := mastodon.NewClient(&mastodon.Config{
            Server:       server,
            ClientID:     app.ClientID,
            ClientSecret: app.ClientSecret,
            AccessToken:  cookie.Value,
        })

        ctx := context.Background()
        if _, err := client.Favourite(ctx, mastodon.ID(req.ID)); err != nil {
            http.Error(w, "Failed to favourite", http.StatusInternalServerError)
            return
        }

        w.WriteHeader(http.StatusOK)
    })

    // Start the web server
    fmt.Println("Server is running on http://localhost:8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
