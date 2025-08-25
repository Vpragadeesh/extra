package main

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/mdp/qrterminal/v3"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
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

	// 2) Fiber app & routes
	app := fiber.New()

	// Serve SPA static files (index.html)
	app.Static("/", staticdir, fiber.Static{
		Index: "index.html",
	})

	// Serve uploaded files under /files/*
	app.Static("/files", uploaddir)

	// Upload endpoint
	app.Post("/upload", uploadHandler)

	// JSON API endpoints
	app.Get("/api/files", filesAPIHandler)
	app.Get("/api/network-info", networkInfoHandler)

	// 3) Print access URLs + QR code
	ip := getLocalIP()
	fmt.Println("Server started at:")
	fmt.Printf("→ http://localhost:%s/\n", serverport)
	if ip != "" {
		url := fmt.Sprintf("http://%s:%s/", ip, serverport)
		fmt.Printf("→ %s\n", url)
		qrterminal.Generate(url, qrterminal.L, os.Stdout)
	}

	// 4) Start Fiber server
	if err := app.Listen(":" + serverport); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

// uploadHandler saves an incoming file then redirects back to "/"
func uploadHandler(c *fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Error reading file: " + err.Error())
	}

	src, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Error opening uploaded file: " + err.Error())
	}
	defer src.Close()

	dstPath := filepath.Join(uploaddir, filepath.Base(fileHeader.Filename))
	dst, err := os.Create(dstPath)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Unable to create file: " + err.Error())
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Error saving file: " + err.Error())
	}

	return c.Redirect("/", fiber.StatusSeeOther)
}

// filesAPIHandler returns the list of uploaded filenames as JSON
func filesAPIHandler(c *fiber.Ctx) error {
	entries, err := os.ReadDir(uploaddir)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Unable to read uploads: " + err.Error())
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return c.JSON(names)
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

// networkInfoHandler returns network information as JSON
func networkInfoHandler(c *fiber.Ctx) error {
	clientIP := getClientIP(c)
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

	return c.JSON(networkInfo)
}

// getClientIP extracts the client IP from the fiber context
func getClientIP(c *fiber.Ctx) string {
	xForwardedFor := c.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		ips := strings.Split(xForwardedFor, ",")
		return strings.TrimSpace(ips[0])
	}

	xRealIP := c.Get("X-Real-IP")
	if xRealIP != "" {
		return xRealIP
	}

	// fiber.Ctx.IP() will try headers and remote address
	if ip := c.IP(); ip != "" {
		return ip
	}

	return "unknown"
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
