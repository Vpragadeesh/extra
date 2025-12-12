package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "github.com/mdp/qrterminal/v3"
    "io"
    "mime"
    "mime/multipart"
    "net"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"
)

const (
    uploaddir  = "/home/pragadeesh/Videos/"
    staticdir  = "/home/pragadeesh/Documents/file-transfer/static"
    serverport = "1234"
)

func main() {
    // 1) Ensure uploads directory exists
    if _, err := os.Stat(uploaddir); os.IsNotExist(err) {
        if err := os.Mkdir(uploaddir, 0755); err != nil {
            fmt.Fprintf(os.Stderr, "Error creating upload directory: %v\n", err)
            os.Exit(1)
        }
    }

    // 2) Setup routes
    mux := http.NewServeMux()

    // Serve static files (index.html)
    mux.Handle("/", http.FileServer(http.Dir(staticdir)))

    // Serve uploaded files under /files/*
    mux.Handle("/files/", http.StripPrefix("/files/", http.FileServer(http.Dir(uploaddir))))

    // Upload endpoint
    mux.HandleFunc("/upload", uploadHandler)

    // JSON API endpoints
    mux.HandleFunc("/api/files", filesAPIHandler)
    mux.HandleFunc("/api/network-info", networkInfoHandler)

    // 3) Print access URLs + QR code
    ip := getLocalIP()
    fmt.Println("Server started at:")
    fmt.Printf("→ http://localhost:%s/\n", serverport)
    if ip != "" {
        url := fmt.Sprintf("http://%s:%s/", ip, serverport)
        fmt.Printf("→ %s\n", url)
        qrterminal.Generate(url, qrterminal.L, os.Stdout)
    }

    // 4) Create server with timeouts
    server := &http.Server{
        Addr:         ":" + serverport,
        Handler:      mux,
        ReadTimeout:  60 * time.Minute,
        WriteTimeout: 60 * time.Minute,
        IdleTimeout:  60 * time.Minute,
    }

    // 5) Start HTTP server
    if err := server.ListenAndServe(); err != nil {
        fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
        os.Exit(1)
    }
}

// uploadHandler saves incoming files directly to disk (streaming, no RAM buffering)
func uploadHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    contentType := r.Header.Get("Content-Type")
    mediaType, params, err := mime.ParseMediaType(contentType)
    if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
        jsonError(w, "Invalid content type", http.StatusBadRequest)
        return
    }

    boundary := params["boundary"]
    if boundary == "" {
        jsonError(w, "No boundary in content type", http.StatusBadRequest)
        return
    }

    // Create multipart reader from request body - streams directly, no RAM buffering
    reader := multipart.NewReader(r.Body, boundary)

    uploadedFiles := make([]string, 0)

    for {
        part, err := reader.NextPart()
        if err == io.EOF {
            break
        }
        if err != nil {
            jsonError(w, "Error reading multipart: "+err.Error(), http.StatusBadRequest)
            return
        }

        // Skip non-file parts
        if part.FileName() == "" {
            part.Close()
            continue
        }

        filename := filepath.Base(part.FileName())
        dstPath := filepath.Join(uploaddir, filename)

        // Create destination file
        dst, err := os.Create(dstPath)
        if err != nil {
            part.Close()
            jsonError(w, "Unable to create file: "+err.Error(), http.StatusInternalServerError)
            return
        }

        // Stream directly from network to disk using buffered writer
        writer := bufio.NewWriterSize(dst, 32*1024*1024) // 32MB buffer
        _, err = io.Copy(writer, part)

        // Flush and close
        writer.Flush()
        dst.Close()
        part.Close()

        if err != nil {
            os.Remove(dstPath) // Clean up partial file
            jsonError(w, "Error saving file: "+err.Error(), http.StatusInternalServerError)
            return
        }

        uploadedFiles = append(uploadedFiles, filename)
    }

    if len(uploadedFiles) == 0 {
        jsonError(w, "No files uploaded", http.StatusBadRequest)
        return
    }

    // Check if request expects JSON response (AJAX) or redirect (form submit)
    accept := r.Header.Get("Accept")
    xhr := r.Header.Get("X-Requested-With") == "XMLHttpRequest"

    if strings.Contains(accept, "application/json") || xhr {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "success": true,
            "message": fmt.Sprintf("%d file(s) uploaded successfully", len(uploadedFiles)),
            "files":   uploadedFiles,
        })
        return
    }

    http.Redirect(w, r, "/", http.StatusSeeOther)
}

// filesAPIHandler returns the list of uploaded files with metadata as JSON
func filesAPIHandler(w http.ResponseWriter, r *http.Request) {
    entries, err := os.ReadDir(uploaddir)
    if err != nil {
        jsonError(w, "Unable to read uploads: "+err.Error(), http.StatusInternalServerError)
        return
    }

    type FileInfo struct {
        Name         string `json:"name"`
        Size         int64  `json:"size"`
        ModifiedTime string `json:"modifiedTime"`
    }

    files := make([]FileInfo, 0, len(entries))
    for _, e := range entries {
        if !e.IsDir() {
            info, err := e.Info()
            if err != nil {
                continue
            }
            files = append(files, FileInfo{
                Name:         e.Name(),
                Size:         info.Size(),
                ModifiedTime: info.ModTime().Format(time.RFC3339),
            })
        }
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(files)
}

// networkInfoHandler returns network information as JSON
func networkInfoHandler(w http.ResponseWriter, r *http.Request) {
    clientIP := getClientIP(r)
    serverIP := getLocalIP()
    if serverIP == "" {
        serverIP = "localhost"
    }
    networkInterface := getNetworkInterface()

    networkInfo := map[string]interface{}{
        "clientIP":         clientIP,
        "serverIP":         serverIP,
        "serverPort":       serverport,
        "protocol":         "HTTP/TCP",
        "networkInterface": networkInterface,
        "connectionType":   "TCP",
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(networkInfo)
}

// jsonError sends a JSON error response
func jsonError(w http.ResponseWriter, message string, status int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// getLocalIP finds the first non-loopback IPv4 address
func getLocalIP() string {
    addrs, err := net.InterfaceAddrs()
    if err != nil {
        return ""
    }
    for _, addr := range addrs {
        if ipnet, ok := addr.(*net.IPNet); ok &&
            !ipnet.IP.IsLoopback() &&
            ipnet.IP.To4() != nil {
            return ipnet.IP.String()
        }
    }
    return ""
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
    xForwardedFor := r.Header.Get("X-Forwarded-For")
    if xForwardedFor != "" {
        ips := strings.Split(xForwardedFor, ",")
        return strings.TrimSpace(ips[0])
    }

    xRealIP := r.Header.Get("X-Real-IP")
    if xRealIP != "" {
        return xRealIP
    }

    ip, _, err := net.SplitHostPort(r.RemoteAddr)
    if err != nil {
        return r.RemoteAddr
    }
    return ip
}

// getNetworkInterface returns information about the active network interface
func getNetworkInterface() string {
    interfaces, err := net.Interfaces()
    if err != nil {
        return "unknown"
    }

    for _, iface := range interfaces {
        if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
            continue
        }

        addrs, err := iface.Addrs()
        if err != nil {
            continue
        }

        for _, addr := range addrs {
            if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
                return fmt.Sprintf("%s (%s)", iface.Name, ipnet.IP.String())
            }
        }
    }

    return "unknown"
}
