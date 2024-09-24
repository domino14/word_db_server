package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/golang-jwt/jwt/v5"
)

type card struct {
	alphagram string
	sols      []string
}

type gameStateManager struct {
	visibleCard card
	loggedIn    bool
	username    string
	jwt         string
}

type loginPacket struct {
	username string
	jwt      string
}

func (m *gameStateManager) View() string {
	if !m.loggedIn {
		return "You are not logged in. Hit enter to open an Aerolith log-in window."
	}

	return "You are logged in as " + m.username
}

type model struct {
	textInput    textinput.Model
	mgr          *gameStateManager
	aerolithURI  string
	callbackChan chan string
}

func initialModel(aerolithURI string) model {
	ti := textinput.New()
	ti.Placeholder = "Guess"
	ti.Focus()
	ti.CharLimit = 20
	ti.Width = 20

	gameStateManager := new(gameStateManager)
	// Register the callback handler here, only once
	callbackChan := make(chan string)

	// Register the callback handler once
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token != "" {
			fmt.Fprintf(w, "Login successful! You can close this window.")
			// Send the token back through the callbackChan
			callbackChan <- token
		} else {
			http.Error(w, "Token not found", http.StatusBadRequest)
		}
	})

	return model{
		textInput:    ti,
		mgr:          gameStateManager,
		aerolithURI:  aerolithURI,
		callbackChan: callbackChan,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {

	// Is it a key press?
	case tea.KeyMsg:

		// Cool, what was the actual key pressed?
		switch msg.Type {

		// These keys should exit the program.
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyEnter:
			if !m.mgr.loggedIn {
				return m, loginCmd(m.aerolithURI, m.mgr, m.callbackChan)
			}
			m.textInput.Reset()
			return m, nil
		}

	// Handle the JWT returned from the loginCmd?
	case loginPacket:
		// JWT received, update the state
		m.mgr.loggedIn = true
		m.mgr.username = msg.username
		m.mgr.jwt = msg.jwt

	case string:
		log.Printf("Possible error: " + msg)

	}
	m.textInput, cmd = m.textInput.Update(msg)

	return m, cmd
}

func (m model) View() string {
	cardview := m.mgr.View()
	return fmt.Sprintf("%s\n\n%s\n\n", cardview, m.textInput.View())
}

// Cmd to handle login logic
func loginCmd(aerolithURI string, mgr *gameStateManager, callbackChan chan string) tea.Cmd {
	return func() tea.Msg {
		// Create a new server instance
		server := &http.Server{Addr: ":8521"}

		serverShutdownChan := make(chan struct{})

		// Start local server
		go startCallbackServer(server, serverShutdownChan)

		// Open the browser to the login page
		openBrowser(fmt.Sprintf("%s/jwt?callback=http://localhost:8521/callback", aerolithURI))

		// Wait for JWT or timeout
		select {
		case loginjwt := <-callbackChan:
			// Parse the JWT token
			p := jwt.NewParser()
			claims := jwt.MapClaims{}
			// As the client we don't need to (and can't) verify the signature of the
			// jwt.
			_, _, err := p.ParseUnverified(loginjwt, &claims)

			if err != nil {
				return "Invalid token. Please log in again." + err.Error()
			}
			var username string
			var ok bool
			// Extract the username from the claims
			if username, ok = claims["usn"].(string); !ok {
				return "Invalid username claim. Please report this."
			}

			mgr.loggedIn = true
			// Gracefully shutdown the server
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(ctx); err != nil {
				log.Printf("Server shutdown failed:%+v", err)
			}
			close(serverShutdownChan)
			return loginPacket{
				username: username,
				jwt:      loginjwt,
			}
		case <-time.After(60 * time.Second):
			log.Println("Login timed out.")
			// Gracefully shutdown the server
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(ctx); err != nil {
				log.Printf("Server shutdown failed:%+v", err)
			}
			close(serverShutdownChan)
			return nil
		}
	}
}

// Start a callback server to receive the JWT
func startCallbackServer(server *http.Server, shutdownChan <-chan struct{}) {
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not listen on :8080: %v\n", err)
		}
	}()

	// Wait for shutdown signal
	<-shutdownChan
}

// Open the browser to the login page
func openBrowser(url string) {
	var err error

	switch os := runtime.GOOS; os {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Fatalf("Failed to open browser: %v", err)
	}
}

func main() {
	aerolithURI := os.Getenv("AEROLITH_URI")
	if aerolithURI == "" {
		aerolithURI = "https://aerolith.org"
	}
	p := tea.NewProgram(initialModel(aerolithURI))

	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
