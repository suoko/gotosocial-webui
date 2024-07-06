package main

import (
    "context"
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

        // Define the HTML template for loading message
        loadingTmpl := template.Must(template.New("loading").Parse(`
            <!DOCTYPE html>
            <html>
            <head>
                <title>Loading...</title>
                <style>
                    body { font-family: Arial, sans-serif; margin: 20px; text-align: center; }
                </style>
            </head>
            <body>
                <h2>Loading data...</h2>
            </body>
            </html>
        `))
        loadingTmpl.Execute(w, nil)

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
                    li { margin-bottom: 20px; padding: 10px; border: 1px solid #ddd; border-radius: 5px; }
                    li:nth-child(odd) { background-color: #f9f9f9; }
                </style>
            </head>
            <body>
                <h2>Gotosocial Home Timeline</h2>
                <ul>
                    {{range .Timeline}}
                        <li><strong>{{.Account.Username}}</strong>: {{safeHTML .Content}}</li>
                    {{end}}
                </ul>
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

    // Start the web server
    fmt.Println("Server is running on http://localhost:8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
