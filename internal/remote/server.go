package remote

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"net"

	"health-monitor/internal/audit"
	"health-monitor/internal/config"
	"health-monitor/internal/output"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/google/uuid"
)

// Commander defines the interface for executing incident actions remotely
type Commander interface {
	AddNote(id, message, user string) error
	Acknowledge(id, user string) error
	Resolve(id, user string, analysis *audit.IncidentAnalysis) error
	AddActionItem(incidentID, description, owner string, priority audit.ActionItemPriority, status audit.ActionItemStatus, dueDate time.Time) error
	Describe(id string) (string, error)
	Suggest(id string) (string, error)
	Similar(id string) (string, error)
}

// Server handles collaborative SSH sessions
type Server struct {
	Addr       string
	Port       int
	Host       string // Override for public address
	IncidentID string
	Token      string // Auth token
	Cmd        Commander
	RenderFunc func(width int) string
	sessions   map[ssh.Session]chan string
	onUpdate   func(string)
	mu         sync.Mutex
}

// NewServer creates a new collaborative server
func NewServer(addr string, port int, id string, cmd Commander, render func(width int) string) *Server {
	// Generate a secure one-time join token
	token := strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
	return &Server{
		Addr:       addr,
		Port:       port,
		IncidentID: id,
		Token:      token,
		Cmd:        cmd,
		RenderFunc: render,
		sessions:   make(map[ssh.Session]chan string),
	}
}

// OnUpdate sets a callback to be triggered when a broadcast occurs
func (s *Server) OnUpdate(f func(string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onUpdate = f
}

// Start launches the SSH server
func (s *Server) Start(ctx context.Context) error {
	pm := config.GetProfileManager()
	keyPath := filepath.Join(pm.GetStatePath(), "remote_id_ed25519")

	srv, err := wish.NewServer(
		wish.WithAddress(fmt.Sprintf("%s:%d", s.Addr, s.Port)),
		wish.WithHostKeyPath(keyPath),
		wish.WithMiddleware(
			func(h ssh.Handler) ssh.Handler {
				return func(sess ssh.Session) {
					// Register session for broadcasts
					bcChan := make(chan string, 10)
					s.mu.Lock()
					s.sessions[sess] = bcChan
					s.mu.Unlock()
					
					defer func() {
						s.mu.Lock()
						delete(s.sessions, sess)
						s.mu.Unlock()
					}()

					// 🛡️ Disable mouse tracking garbage
					wish.Print(sess, "\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l")

					h(sess)
				}
			},
			bubbletea.Middleware(s.Handler),
		),
		wish.WithPasswordAuth(func(_ ssh.Context, password string) bool {
			return password == s.Token
		}),
	)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	go func() {
		output.Infof("Collaborative server listening on %s:%d", s.Addr, s.Port)
		if err := srv.ListenAndServe(); err != nil && err != ssh.ErrServerClosed {
			output.Errorf("SSH server error: %v", err)
		}
		close(done)
	}()

	select {
	case <-ctx.Done():
		output.Infof("Stopping collaborative server...")
		_ = srv.Close()
	case <-done:
	}

	return nil
}

func (s *Server) Handler(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
	s.mu.Lock()
	bcChan := s.sessions[sess]
	s.mu.Unlock()

	intro := fmt.Sprintf("Welcome to Health Monitor Collaborative Session!\n" +
		"--------------------------------------------\n" +
		"You are now connected to the live session.\n")

	m := &remoteModel{
		sess:       sess,
		incidentID: s.IncidentID,
		cmd:        s.Cmd,
		server:     s,
		input:      textinput.New(),
		welcome:    intro,
		broadcasts: bcChan,
	}
	m.input.Placeholder = "Enter command... (view, note, ack, resolve, help)"
	m.input.Focus()

	return m, []tea.ProgramOption{tea.WithAltScreen()}
}

func (s *Server) GetConnectionInfo() string {
	ip := s.Host
	if ip == "" {
		ip = s.discoverPublicIP()
	}
	if ip == "" {
		ip = s.getOutboundIP()
	}
	// Return the full connection guidance
	return fmt.Sprintf("Collaborative Session Active | Host: %s | Port: %d | Token: %s | Join: ssh %s -p %d", 
		ip, s.Port, s.Token, ip, s.Port)
}

func (s *Server) discoverPublicIP() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// icanhazip.com is a reliable service for finding public IP
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", "icanhazip.com:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: icanhazip.com\r\nConnection: close\r\n\r\n")
	
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return ""
	}
	
	resp := string(buf[:n])
	lines := strings.Split(resp, "\n")
	if len(lines) > 0 {
		ip := strings.TrimSpace(lines[len(lines)-1])
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	return ""
}

func (s *Server) getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "localhost"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func (s *Server) Broadcast(msg string) {
	s.mu.Lock()
	onUpd := s.onUpdate
	s.mu.Unlock()

	if onUpd != nil {
		onUpd(msg)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.sessions {
		select {
		case ch <- msg:
		default:
			// Buffer full, skip
		}
	}
}

type broadcastMsg string

func waitForBroadcast(ch chan string) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return nil
		}
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return broadcastMsg(msg)
	}
}

type inputType int

const (
	inputNone inputType = iota
	inputNote
	inputResolveSummary
	inputResolveRootCause
	inputResolveFix
	inputResolveComponent
	inputResolveCategory
	inputResolveDependency
	inputResolveFailureType
	inputResolvePrevention
	inputResolveWell
	inputResolveBetter
	inputResolveLucky
	inputResolveDowntime
	inputResolveToilMin
	inputResolveToilCat
	inputActionDesc
	inputActionOwner
	inputActionPriority
	inputActionDueDate
	inputActionStatus
)

type remoteModel struct {
	sess       ssh.Session
	incidentID string
	cmd        Commander
	server     *Server
	viewport   viewport.Model
	input      textinput.Model
	welcome    string
	history    string
	err        error
	ready      bool
	width      int
	height     int
	broadcasts chan string
	
	// Wizard fields
	activeInput inputType
	wizardData  map[string]string
	wizardSteps []inputType
	currentStep int
}

func (m *remoteModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		waitForBroadcast(m.broadcasts),
	)
}

func (m *remoteModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case broadcastMsg:
		ts := time.Now().Format("15:04:05")
		m.history = fmt.Sprintf("[%s] %s\n%s", ts, string(msg), m.history)
		cmds = append(cmds, waitForBroadcast(m.broadcasts))
		// Real-time refresh: update viewport content on broadcast
		if m.ready {
			content := m.server.RenderFunc(m.width)
			m.viewport.SetContent(m.welcome + "\n" + content)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		
		vHeight := msg.Height - 12 // Sufficient room for history and prompt
		
		if !m.ready {
			m.viewport = viewport.New(msg.Width, vHeight)
			content := m.server.RenderFunc(msg.Width)
			m.viewport.SetContent(m.welcome + "\n" + content)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = vHeight
			content := m.server.RenderFunc(msg.Width)
			m.viewport.SetContent(m.welcome + "\n" + content)
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			val := m.input.Value()
			if m.activeInput != inputNone {
				m.wizardData[m.input.Placeholder] = val
				m.input.SetValue("")
				m.currentStep++
				if m.currentStep < len(m.wizardSteps) {
					m.activeInput = m.wizardSteps[m.currentStep]
					m.input.Placeholder = m.getPlaceholder(m.activeInput)
					return m, nil
				}
				
				err := m.finishWizard()
				if err != nil {
					m.err = err
				}
				m.activeInput = inputNone
				m.input.Placeholder = "Enter command..."
				m.input.Focus() // Ensure focused for next command
				
				// Refresh viewport after wizard
				if m.ready {
					content := m.server.RenderFunc(m.width)
					m.viewport.SetContent(m.welcome + "\n" + content)
				}
				return m, nil
			}
			
			m.input.SetValue("")
			if val != "" {
				cmds = append(cmds, m.handleCommand(val))
				// Immediate viewport refresh after command
				if m.ready {
					content := m.server.RenderFunc(m.width)
					m.viewport.SetContent(m.welcome + "\n" + content)
				}
			}
		case "esc":
			if m.activeInput != inputNone {
				m.activeInput = inputNone
				m.input.Placeholder = "Enter command..."
				m.input.Blur()
				m.input.SetValue("")
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	if m.ready {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *remoteModel) getPlaceholder(it inputType) string {
	switch it {
	case inputNote: return "Note message"
	case inputResolveSummary: return "Summary"
	case inputResolveRootCause: return "Root Cause"
	case inputResolveFix: return "Fix"
	case inputResolveComponent: return "Component (e.g., api, database)"
	case inputResolveCategory: return "Category (capacity, latency, dependency, config, infra, deployment, security)"
	case inputResolveDependency: return "Dependency (e.g., api->db)"
	case inputResolveFailureType: return "Failure Type (service, dependency, infra)"
	case inputResolvePrevention: return "Prevention Measures"
	case inputResolveWell: return "What went well"
	case inputResolveBetter: return "What could be better"
	case inputResolveLucky: return "Where we got lucky"
	case inputResolveDowntime: return "Downtime (minutes)"
	case inputResolveToilMin: return "Toil/Manual Effort (minutes)"
	case inputResolveToilCat: return "Toil Category (manual_restart, config_change, etc.)"
	case inputActionDesc: return "Description (e.g., Fix database retry logic)"
	case inputActionOwner: return "Owner (e.g., sanjana)"
	case inputActionPriority: return "Priority (P1/P2/P3/P4) [P2]"
	case inputActionDueDate: return "Due date (YYYY-MM-DD) [2026-03-26]"
	case inputActionStatus: return "Status (TODO/IN_PROGRESS/DONE/WONT_FIX) [TODO]"
	}
	return "Enter value..."
}

func (m *remoteModel) finishWizard() error {
	p := m.sess.User()
	switch m.wizardSteps[0] {
	case inputResolveSummary:
		// Not ideal to recreate the details here but Commander is limited
		// I should ideally add a more robust Resolve method	case inputResolveSummary:
		analysis := &audit.IncidentAnalysis{
			RootCause:  m.wizardData["Root Cause"],
			FixSummary: m.wizardData["Fix"],
			Component:  m.wizardData["Component (e.g., api, database)"],
			Category:   m.wizardData["Category (capacity, latency, dependency, config, infra, deployment, security)"],
			Dependency: m.wizardData["Dependency (e.g., api->db)"],
			FailureType: m.wizardData["Failure Type (service, dependency, infra)"],
			Prevention: m.wizardData["Prevention Measures"],
			LessonsLearned: &audit.LessonsLearned{
				WhatWentWell:      m.wizardData["What went well"],
				WhatCouldBeBetter: m.wizardData["What could be better"],
				WhereWeGotLucky:   m.wizardData["Where we got lucky"],
			},
		}
		summary := m.wizardData["Summary"]
		if summary == "" { summary = "Resolved via collaborative session" }
		
		if err := m.cmd.Resolve(m.incidentID, p, analysis); err == nil {
			m.history = "✓ Incident resolved"
			m.server.Broadcast(fmt.Sprintf("%s resolved the incident", p))
		} else {
			return err
		}
	case inputActionDesc:
		desc := m.wizardData["Description (e.g., Fix database retry logic)"]
		owner := m.wizardData["Owner (e.g., sanjana)"]
		if owner == "" { owner = p }
		
		priorityStr := m.wizardData["Priority (P1/P2/P3/P4) [P2]"]
		priority, err := audit.ParseActionItemPriority(priorityStr)
		if err != nil {
			return err
		}
		
		dueDateStr := m.wizardData["Due date (YYYY-MM-DD) [2026-03-26]"]
		if dueDateStr == "" { dueDateStr = "2026-03-26" }
		dueDate, _ := time.Parse("2006-01-02", dueDateStr)
		
		statusStr := m.wizardData["Status (TODO/IN_PROGRESS/DONE/WONT_FIX) [TODO]"]
		status, err := audit.ParseActionItemStatus(statusStr)
		if err != nil {
			return err
		}
		
		if err := m.cmd.AddActionItem(m.incidentID, desc, owner, priority, status, dueDate); err == nil {
			m.history = fmt.Sprintf("✓ Action item added: %s", desc)
			m.server.Broadcast(fmt.Sprintf("%s added action item: %s", p, desc))
		} else {
			return err
		}
	case inputNote:
		msg := m.wizardData["Note message"]
		if err := m.cmd.AddNote(m.incidentID, msg, p); err == nil {
			m.history = "✓ Note added"
			m.server.Broadcast(fmt.Sprintf("%s added a note: %s", p, msg))
		} else {
			return err
		}
	}
	return nil
}

func (m *remoteModel) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	var sb strings.Builder
	sb.WriteString(m.viewport.View())
	sb.WriteString("\n\n")
	
	if m.history != "" {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true).Render("Operational History:"))
		sb.WriteString("\n" + m.history)
	}
	
	if m.err != nil {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("\n✗ Error: %v", m.err)) + "\n")
	}
	
	if m.activeInput != inputNone {
		header := fmt.Sprintf("\x1b[1;35m%s (Step %d/%d)\x1b[0m", m.input.Placeholder, m.currentStep+1, len(m.wizardSteps))
		sb.WriteString("\n" + header + "\n")
	}

	sb.WriteString(fmt.Sprintf("\n\x1b[1;36m[%s]\x1b[0m # %s", m.sess.User(), m.input.View()))
	return sb.String()
}

func (m *remoteModel) handleCommand(line string) tea.Cmd {
	parts := strings.Fields(stripAnsi(line))
	if len(parts) == 0 {
		return nil
	}
	p := m.sess.User()
	cmd := strings.ToLower(parts[0])

	return func() tea.Msg {
		switch cmd {
		case "exit", "quit":
			return tea.Quit()
		case "help", "/?":
			m.history = "Available Commands: view, note, ack, resolve, action add, suggest, similar, replay, exit"
		case "view":
			// Refresh stored incident data might be needed here too if we want "view" to be fresh
			m.history = "Refreshed view."
		case "ack":
			if err := m.cmd.Acknowledge(m.incidentID, p); err != nil {
				m.err = err
			} else {
				m.history = "✓ Incident acknowledged"
				m.server.Broadcast(fmt.Sprintf("%s acknowledged the incident", p))
			}
		case "suggest":
			res, _ := m.cmd.Suggest(m.incidentID)
			m.history = "AI Suggestion:\n" + res
		case "similar":
			res, _ := m.cmd.Similar(m.incidentID)
			m.history = "Similar Incidents:\n" + res
		case "resolve":
			m.activeInput = inputResolveSummary
			m.wizardSteps = []inputType{
				inputResolveSummary, inputResolveRootCause, inputResolveFix, 
				inputResolveComponent, inputResolveCategory, inputResolveDependency, 
				inputResolveFailureType, inputResolvePrevention, inputResolveWell, 
				inputResolveBetter, inputResolveLucky, inputResolveDowntime, 
				inputResolveToilMin, inputResolveToilCat,
			}
			m.wizardData = make(map[string]string)
			m.currentStep = 0
			m.input.Placeholder = m.getPlaceholder(m.activeInput)
			m.input.Focus()
		case "action":
			if len(parts) > 1 && parts[1] == "add" {
				m.activeInput = inputActionDesc
				m.wizardSteps = []inputType{inputActionDesc, inputActionOwner, inputActionPriority, inputActionDueDate, inputActionStatus}
				m.wizardData = make(map[string]string)
				m.currentStep = 0
				m.input.Placeholder = m.getPlaceholder(m.activeInput)
				m.input.Focus()
			} else {
				m.history = "Usage: action add"
			}
		case "replay":
			history, _ := audit.GetHistory(m.incidentID)
			var sb strings.Builder
			for i, a := range history {
				if i > 5 { break } // Limit replay
				sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", a.Timestamp.Format("15:04:05"), a.User, a.Activity))
			}
			m.history = "Direct Audit Replay:\n" + sb.String()
		case "note":
			msg := strings.Join(parts[1:], " ")
			if msg == "" {
				m.history = "Usage: note <message>"
			} else {
				if err := m.cmd.AddNote(m.incidentID, msg, p); err != nil {
					m.err = err
				} else {
					m.history = "✓ Note added"
					m.server.Broadcast(fmt.Sprintf("%s added a note: %s", p, msg))
				}
			}
		default:
			m.history = fmt.Sprintf("Unknown command: %s", cmd)
		}
		return nil
	}
}

func stripAnsi(input string) string {
	var result strings.Builder
	skip := false
	for i := 0; i < len(input); i++ {
		if input[i] == 27 { // ESC
			skip = true
			continue
		}
		if skip {
			if (input[i] >= 'a' && input[i] <= 'z') || (input[i] >= 'A' && input[i] <= 'Z') || input[i] == ' ' || input[i] == 'm' {
				skip = false
			}
			continue
		}
		result.WriteByte(input[i])
	}
	return strings.TrimSpace(result.String())
}
