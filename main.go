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

var (
    mu    sync.Mutex
    server string
    app   *mastodon.Application
)

type PageData struct {
    Timeline []*mastodon.Status
    Server   string
    Code     string
    NextLink string
    PrevLink string
}

func main() {
    http.HandleFunc("/", homeHandler)
    http.HandleFunc("/callback", callbackHandler)
    http.HandleFunc("/reply", replyHandler)
    http.HandleFunc("/boost", boostHandler)
    http.HandleFunc("/favourite", favouriteHandler)

    // Serve static files for CSS and JavaScript
    http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

    fmt.Println("Server is running on http://localhost:8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
    tokenCookie, err := r.Cookie("access_token")
    serverCookie, err := r.Cookie("server")
    if err != nil || tokenCookie == nil || serverCookie == nil {
        instance := r.FormValue("instance")
        if instance == "" {
            tmpl := template.Must(template.ParseFiles("templates/instance.html"))
            tmpl.Execute(w, nil)
            return
        }

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

        mu.Lock()
        server = instance
        mu.Unlock()

        authURL := app.AuthURI
        http.Redirect(w, r, authURL, http.StatusFound)
        return
    }

    accessToken := tokenCookie.Value
    server := serverCookie.Value
    client := mastodon.NewClient(&mastodon.Config{
        Server:      server,
        AccessToken: accessToken,
    })

    ctx := context.Background()
    timeline, err := client.GetTimelineHome(ctx, nil)
    if err != nil {
        http.Error(w, "Failed to fetch timeline", http.StatusInternalServerError)
        log.Println("Failed to fetch timeline:", err)
        return
    }

    log.Println("About to parse template")

    // Directly parse the files without template.New
    tmpl, err := template.New("timeline").Funcs(template.FuncMap{
        "safeHTML": func(text string) template.HTML {
            return template.HTML(text)
        },
        "hasMedia": func(status *mastodon.Status) bool {
            return len(status.MediaAttachments) > 0
        },
    }).ParseFiles("templates/timeline.html")

    if err != nil {
        http.Error(w, "Failed to load template", http.StatusInternalServerError)
        log.Printf("Failed to load template: %v", err)
        return
    }

    log.Println("Template parsed successfully")

    // Ensure we have data
    log.Printf("Timeline length: %d", len(timeline))

    data := PageData{Timeline: timeline, Server: server, PrevLink: "", NextLink: ""}
    if err := tmpl.ExecuteTemplate(w, "timeline.html", data); err != nil {
        http.Error(w, "Failed to render template", http.StatusInternalServerError)
        log.Printf("Failed to render template: %v", err)
    }
}





func callbackHandler(w http.ResponseWriter, r *http.Request) {
    code := r.URL.Query().Get("code")
    if code == "" {
        http.Error(w, "Authorization code not found", http.StatusBadRequest)
        return
    }

    mu.Lock()
    localServer := server
    localApp := app
    mu.Unlock()

    if localApp == nil {
        http.Error(w, "Application not registered", http.StatusInternalServerError)
        log.Println("Application not registered")
        return
    }

    client := mastodon.NewClient(&mastodon.Config{
        Server:      localServer,
        ClientID:    localApp.ClientID,
        ClientSecret: localApp.ClientSecret,
    })

    err := client.AuthenticateToken(context.Background(), code, "http://localhost:8080/callback")
    if err != nil {
        http.Error(w, "Failed to authenticate", http.StatusInternalServerError)
        log.Fatalf("Error authenticating: %v", err)
        return
    }

    http.SetCookie(w, &http.Cookie{
        Name:   "access_token",
        Value:  client.Config.AccessToken,
        Expires: time.Now().Add(24 * time.Hour),
        Path:   "/",
    })
    http.SetCookie(w, &http.Cookie{
        Name:   "server",
        Value:  localServer,
        Expires: time.Now().Add(24 * time.Hour),
        Path:   "/",
    })

    http.Redirect(w, r, "/", http.StatusFound)
}

func replyHandler(w http.ResponseWriter, r *http.Request) {
    var req struct {
        ID       string `json:"id"`
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
        Server:      server,
        ClientID:    app.ClientID,
        ClientSecret: app.ClientSecret,
        AccessToken: cookie.Value,
    })

    ctx := context.Background()
    if _, err := client.PostStatus(ctx, &mastodon.Toot{
        InReplyToID: mastodon.ID(req.ID),
        Status:     req.ReplyText,
        Visibility: mastodon.VisibilityPublic,
    }); err != nil {
        http.Error(w, "Failed to reply", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
}

func boostHandler(w http.ResponseWriter, r *http.Request) {
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
        Server:      server,
        ClientID:    app.ClientID,
        ClientSecret: app.ClientSecret,
        AccessToken: cookie.Value,
    })

    ctx := context.Background()
    if _, err := client.Reblog(ctx, mastodon.ID(req.ID)); err != nil {
        http.Error(w, "Failed to boost", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
}

func favouriteHandler(w http.ResponseWriter, r *http.Request) {
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
        Server:      server,
        ClientID:    app.ClientID,
        ClientSecret: app.ClientSecret,
        AccessToken: cookie.Value,
    })

    ctx := context.Background()
    if _, err := client.Favourite(ctx, mastodon.ID(req.ID)); err != nil {
        http.Error(w, "Failed to favourite", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
}


