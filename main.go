package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// State defines the application mode
type State int

const (
	StateOnboarding State = iota
	StateMain
)

// ProjectType defines the type of PHP project
type ProjectType string

const (
	TypeLaravel ProjectType = "Laravel"
	TypePHP     ProjectType = "PHP Built-in"
)

// Project holds metadata about scanned directories
type Project struct {
	Name   string
	Port   int
	Status string // "OFF" atau "RUNNING"
	Type   ProjectType
	Path   string
	Cmd    *exec.Cmd
}

// Config maps to config.json structure
type Config struct {
	ProjectsDir string `json:"projects_dir"`
	PHPPath     string `json:"php_path"`
}

type model struct {
	state       State
	projects    []Project
	cursor      int
	textInput   textinput.Model
	errorMsg    string
	configPath  string
	phpPath     string
	projectsDir string
}

// Lipgloss styles for rendering
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#38bdf8")).
			Bold(true).
			MarginBottom(1)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#334155")).
			Padding(1, 2).
			Margin(1, 0)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ef4444")).
			Bold(true).
			MarginTop(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#38bdf8")).
			Padding(0, 1)

	cellStyle = lipgloss.NewStyle().
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("#4f46e5")).
			Foreground(lipgloss.Color("#ffffff")).
			Padding(0, 1)

	statusRunningStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#10b981")).
				Bold(true)

	statusOffStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#64748b"))
)

// expandTilde replaces ~ with the user's home directory path
func expandTilde(path string) string {
	if len(path) == 0 {
		return path
	}
	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if len(path) == 1 {
			return home
		}
		if path[1] == '/' {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// detectPHPPath finds PHP in standard Herd-Lite location or user PATH
func detectPHPPath() string {
	home, err := os.UserHomeDir()
	if err == nil {
		herdPath := filepath.Join(home, ".config/herd-lite/bin/php")
		if _, err := os.Stat(herdPath); err == nil {
			return herdPath
		}
	}

	if path, err := exec.LookPath("php"); err == nil {
		return path
	}

	return "php"
}

// getConfigPath determines where config.json should be saved
func getConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appConfigDir := filepath.Join(configDir, "serveproxy")
	if err := os.MkdirAll(appConfigDir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(appConfigDir, "config.json"), nil
}

// loadConfig reads config.json or returns defaults
func loadConfig() (Config, string, error) {
	var cfg Config
	path, err := getConfigPath()
	if err != nil {
		return cfg, "", err
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		cfg.PHPPath = detectPHPPath()
		return cfg, path, err
	}

	err = json.Unmarshal(bytes, &cfg)
	if err != nil {
		cfg.PHPPath = detectPHPPath()
		return cfg, path, err
	}

	if cfg.PHPPath == "" {
		cfg.PHPPath = detectPHPPath()
	}

	return cfg, path, nil
}

// scanProjects lists projects that match Laravel or PHP built-in server criteria
func scanProjects(projectsDir string) ([]Project, error) {
	files, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, err
	}

	var projs []Project
	startPort := 8000

	for _, file := range files {
		if file.IsDir() {
			dirPath := filepath.Join(projectsDir, file.Name())

			// 1. Laravel: has artisan in root
			artisanPath := filepath.Join(dirPath, "artisan")
			if _, err := os.Stat(artisanPath); err == nil {
				projs = append(projs, Project{
					Name:   file.Name(),
					Port:   startPort,
					Status: "OFF",
					Type:   TypeLaravel,
					Path:   dirPath,
				})
				startPort++
				continue
			}

			// 2. PHP Built-in (Public index): has public/index.php
			publicIndexPath := filepath.Join(dirPath, "public/index.php")
			if _, err := os.Stat(publicIndexPath); err == nil {
				projs = append(projs, Project{
					Name:   file.Name(),
					Port:   startPort,
					Status: "OFF",
					Type:   TypePHP,
					Path:   dirPath,
				})
				startPort++
				continue
			}

			// 3. PHP Built-in (Root index): has index.php in root
			indexPath := filepath.Join(dirPath, "index.php")
			if _, err := os.Stat(indexPath); err == nil {
				projs = append(projs, Project{
					Name:   file.Name(),
					Port:   startPort,
					Status: "OFF",
					Type:   TypePHP,
					Path:   dirPath,
				})
				startPort++
				continue
			}
		}
	}

	return projs, nil
}

func (m *model) cleanup() {
	for _, p := range m.projects {
		if p.Status == "RUNNING" && p.Cmd != nil {
			_ = syscall.Kill(-p.Cmd.Process.Pid, syscall.SIGKILL)
		}
	}
	saveNginxMap([]Project{})
}

func (m model) Init() tea.Cmd {
	if m.state == StateOnboarding {
		return textinput.Blink
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cleanup()
			return m, tea.Quit
		}
	}

	if m.state == StateOnboarding {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				val := m.textInput.Value()
				expanded := expandTilde(val)

				info, err := os.Stat(expanded)
				if err != nil || !info.IsDir() {
					m.errorMsg = "❌ Direktori tidak ditemukan atau bukan folder valid!"
					return m, nil
				}

				// Check if readable
				_, err = os.ReadDir(expanded)
				if err != nil {
					m.errorMsg = "❌ Tidak dapat membaca folder: " + err.Error()
					return m, nil
				}

				m.projectsDir = val
				config := Config{
					ProjectsDir: val,
					PHPPath:     m.phpPath,
				}
				configBytes, err := json.MarshalIndent(config, "", "  ")
				if err != nil {
					m.errorMsg = "❌ Gagal mengonversi konfigurasi ke JSON: " + err.Error()
					return m, nil
				}
				err = os.WriteFile(m.configPath, configBytes, 0644)
				if err != nil {
					m.errorMsg = "❌ Gagal menyimpan file konfigurasi: " + err.Error()
					return m, nil
				}

				projs, err := scanProjects(expanded)
				if err != nil {
					m.errorMsg = "❌ Gagal memindai folder proyek: " + err.Error()
					return m, nil
				}

				m.projects = projs
				m.state = StateMain
				m.errorMsg = ""
				return m, nil

			}
		}

		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

	// Main TUI logic
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if len(m.projects) > 0 && m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if len(m.projects) > 0 && m.cursor < len(m.projects)-1 {
				m.cursor++
			}

		case "enter", " ":
			if len(m.projects) == 0 {
				break
			}
			p := &m.projects[m.cursor]
			if p.Status == "OFF" {
				var runCmd *exec.Cmd
				if p.Type == TypeLaravel {
					runCmd = exec.Command(m.phpPath, "artisan", "serve", "--port="+strconv.Itoa(p.Port))
				} else {
					publicIndexPath := filepath.Join(p.Path, "public/index.php")
					if _, err := os.Stat(publicIndexPath); err == nil {
						runCmd = exec.Command(m.phpPath, "-S", "127.0.0.1:"+strconv.Itoa(p.Port), "-t", "public")
					} else {
						runCmd = exec.Command(m.phpPath, "-S", "127.0.0.1:"+strconv.Itoa(p.Port))
					}
				}
				runCmd.Dir = p.Path
				runCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

				err := runCmd.Start()
				if err == nil {
					p.Cmd = runCmd
					p.Status = "RUNNING"
					saveNginxMap(m.projects)
				}
			} else {
				if p.Cmd != nil && p.Cmd.Process != nil {
					_ = syscall.Kill(-p.Cmd.Process.Pid, syscall.SIGKILL)
					p.Status = "OFF"
					p.Cmd = nil
					saveNginxMap(m.projects)
				}
			}
		}
	}

	return m, nil
}

func (m model) viewOnboarding() string {
	var s string
	s += titleStyle.Render("🚀 ServeProxy Onboarding") + "\n"
	s += "Selamat datang di ServeProxy!\n"
	s += "Aplikasi ini memindai folder projects Anda untuk meluncurkan server lokal secara otomatis.\n\n"
	s += "Masukkan path absolut folder proyek Anda (contoh: ~/Projects atau /home/user/Projects):\n\n"
	s += m.textInput.View() + "\n"

	if m.errorMsg != "" {
		s += errorStyle.Render(m.errorMsg) + "\n"
	}

	s += "\nTekan [Enter] untuk menyimpan, atau [Ctrl+C] untuk keluar."
	return boxStyle.Render(s) + "\n"
}

func (m model) viewMain() string {
	s := titleStyle.Render("🚀 SERVEPROXY - Projects Dashboard") + "\n"
	s += fmt.Sprintf("Folder  : %s\n", m.projectsDir)
	s += fmt.Sprintf("PHP Path: %s\n", m.phpPath)
	s += fmt.Sprintf("Config  : %s\n\n", m.configPath)

	if len(m.projects) == 0 {
		s += "  (Tidak ada project Laravel atau PHP di folder ini)\n\n"
	} else {
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#475569"))).
			Headers("", "PROJECT NAME", "TYPE", "PORT", "STATUS", "LOCAL URL")

		for i, p := range m.projects {
			cursorStr := " "
			if m.cursor == i {
				cursorStr = "▸"
			}

			statusStr := "○ OFF"
			if p.Status == "RUNNING" {
				statusStr = "● RUNNING"
			}

			urlStr := "-"
			if p.Status == "RUNNING" {
				urlStr = fmt.Sprintf("http://%s.test", p.Name)
			}

			t.Row(
				cursorStr,
				p.Name,
				string(p.Type),
				strconv.Itoa(p.Port),
				statusStr,
				urlStr,
			)
		}

		t.StyleFunc(func(row, col int) lipgloss.Style {
			var style lipgloss.Style
			if row == table.HeaderRow {
				style = headerStyle
			} else {
				isSelected := row == m.cursor
				if isSelected {
					style = selectedStyle
				} else {
					style = cellStyle
				}

				// Customize status column colors when not selected
				if !isSelected && col == 4 { // Status column
					p := m.projects[row]
					if p.Status == "RUNNING" {
						style = statusRunningStyle.Inherit(style)
					} else {
						style = statusOffStyle.Inherit(style)
					}
				}
			}

			// Apply defined column widths and alignments
			switch col {
			case 0: // cursor
				style = style.Width(3).Align(lipgloss.Center)
			case 1: // PROJECT NAME
				style = style.Width(25).Align(lipgloss.Left)
			case 2: // TYPE
				style = style.Width(15).Align(lipgloss.Left)
			case 3: // PORT
				style = style.Width(8).Align(lipgloss.Center)
			case 4: // STATUS
				style = style.Width(12).Align(lipgloss.Center)
			case 5: // LOCAL URL
				style = style.Width(30).Align(lipgloss.Left)
			}

			return style
		})

		s += t.String() + "\n"
	}

	s += "Navigasi: [↑/↓] atau [k/j] | Aksi: [Enter] / [Spasi] Toggle Server | Keluar: [Ctrl+C]\n"
	return s
}

func (m model) View() string {
	if m.state == StateOnboarding {
		return m.viewOnboarding()
	}
	return m.viewMain()
}

func main() {
	cfg, configPath, _ := loadConfig()

	var state State
	var projs []Project
	var errMsg string

	if cfg.ProjectsDir == "" {
		state = StateOnboarding
	} else {
		projectsDir := expandTilde(cfg.ProjectsDir)
		info, err := os.Stat(projectsDir)
		if err != nil || !info.IsDir() {
			state = StateOnboarding
			errMsg = "⚠️ Folder project saat ini tidak valid. Silakan atur ulang."
		} else {
			state = StateMain
			projs, err = scanProjects(projectsDir)
			if err != nil {
				state = StateOnboarding
				errMsg = "⚠️ Gagal membaca folder project: " + err.Error()
			}
		}
	}

	ti := textinput.New()
	ti.Placeholder = "/home/user/Projects"
	ti.Focus()
	ti.CharLimit = 250
	ti.Width = 50

	if cfg.ProjectsDir != "" {
		ti.SetValue(cfg.ProjectsDir)
	}

	m := model{
		state:       state,
		projects:    projs,
		cursor:      0,
		textInput:   ti,
		errorMsg:    errMsg,
		configPath:  configPath,
		phpPath:     cfg.PHPPath,
		projectsDir: cfg.ProjectsDir,
	}

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Ada error di aplikasi TUI: %v\n", err)
		os.Exit(1)
	}
}

func saveNginxMap(projects []Project) {
	file, err := os.Create("/etc/nginx/tui_ports.map")
	if err != nil {
		return
	}
	defer file.Close()

	for _, p := range projects {
		if p.Status == "RUNNING" {
			fmt.Fprintf(file, "    %s.test %d;\n", p.Name, p.Port)
			fmt.Fprintf(file, "    *.%s.test %d;\n", p.Name, p.Port)
		}
	}

	_ = exec.Command("nginx", "-s", "reload").Run()
}
