package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
	StatePHPVersionPicker
	StateSetDefaultPicker
)

// ProjectType defines the type of PHP project
type ProjectType string

const (
	TypeLaravel ProjectType = "Laravel"
	TypePHP     ProjectType = "PHP Built-in"
)

// Project holds metadata about scanned directories
type Project struct {
	Name       string
	Port       int
	Status     string // "OFF" atau "RUNNING"
	Type       ProjectType
	Path       string
	Cmd        *exec.Cmd
	PHPVersion string // "8.4", "8.3", "8.2", atau "" (default)
}

// Config maps to config.json structure
type Config struct {
	ProjectsDir      string            `json:"projects_dir"`
	PHPPath          string            `json:"php_path"`
	PHPVersions      map[string]string `json:"php_versions,omitempty"`      // project_name → version ("8.4")
	DefaultPHPVersion string           `json:"default_php_version,omitempty"` // default version
}

type model struct {
	state              State
	projects           []Project
	cursor             int
	textInput          textinput.Model
	errorMsg           string
	configPath         string
	phpPath            string
	projectsDir        string
	phpVersions        map[string]string // project_name → version from config
	defaultPHPVersion  string            // configured default version
	defaultPHPDetected string            // actual version of default php binary (e.g. "8.4")
	availableVersions  []string          // detected available PHP versions (e.g. ["8.2","8.3","8.4"])
	pickerCursor       int               // cursor in version picker
	pickerProjectIdx   int               // which project is being edited
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

	phpVersionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#a78bfa")).
				Bold(true)

	phpVersionDefaultStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#64748b")).
				Italic(true)

	availableVersionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#10b981")).
				Bold(true)

	pickerTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#38bdf8")).
				Bold(true).
				MarginBottom(1)

	pickerSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#4f46e5")).
				Foreground(lipgloss.Color("#ffffff")).
				Padding(0, 1)

	pickerItemStyle = lipgloss.NewStyle().
				Padding(0, 1)

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f59e0b")).
			Bold(true).
			MarginTop(1)
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

// detectAvailablePHPVersions scans ~/.config/herd-lite/bin/ for php* binaries
// and returns sorted list of version strings (e.g. ["8.2","8.3","8.4"])
func detectAvailablePHPVersions() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	binDir := filepath.Join(home, ".config/herd-lite/bin")
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return nil
	}

	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Match pattern: php8.2, php8.3, php8.4 (not plain "php")
		if len(name) > 3 && name[:3] == "php" && name[3] >= '0' && name[3] <= '9' {
			ver := name[3:] // e.g. "8.2", "8.3", "8.4"
			versions = append(versions, ver)
		}
	}

	// Sort versions: 8.2 < 8.3 < 8.4
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) < 0
	})

	return versions
}

// compareVersions compares two version strings like "8.2" and "8.4"
// returns -1 if a < b, 0 if equal, 1 if a > b
func compareVersions(a, b string) int {
	ai, _ := strconv.Atoi(strings.Split(a, ".")[0])
	bi, _ := strconv.Atoi(strings.Split(b, ".")[0])
	if ai != bi {
		if ai < bi {
			return -1
		}
		return 1
	}
	aj, _ := strconv.Atoi(strings.Split(a, ".")[1])
	bj, _ := strconv.Atoi(strings.Split(b, ".")[1])
	if aj < bj {
		return -1
	}
	if aj > bj {
		return 1
	}
	return 0
}

// detectProjectPHPVersion reads .php-version file from project root
func detectProjectPHPVersion(projectPath string) string {
	versionFile := filepath.Join(projectPath, ".php-version")
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// detectDefaultPHPVersion runs `php -v` and parses the version string (e.g. "8.4")
func detectDefaultPHPVersion(phpPath string) string {
	out, err := exec.Command(phpPath, "-v").Output()
	if err != nil {
		return ""
	}
	// Output format: "PHP 8.4.12 (cli) ..." — extract "8.4"
	lines := strings.SplitN(string(out), "\n", 2)
	if len(lines) == 0 {
		return ""
	}
	parts := strings.Fields(lines[0])
	if len(parts) < 2 {
		return ""
	}
	ver := parts[1] // e.g. "8.4.12"
	dotParts := strings.SplitN(ver, ".", 3)
	if len(dotParts) < 2 {
		return ver
	}
	return dotParts[0] + "." + dotParts[1] // e.g. "8.4"
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

// resolvePHPBinary maps a version string to the actual binary path
func resolvePHPBinary(version string, fallback string) string {
	if version == "" {
		return fallback
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fallback
	}

	versionedPath := filepath.Join(home, ".config/herd-lite/bin/php"+version)
	if _, err := os.Stat(versionedPath); err == nil {
		return versionedPath
	}

	return fallback
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
		cfg.DefaultPHPVersion = ""
		return cfg, path, err
	}

	err = json.Unmarshal(bytes, &cfg)
	if err != nil {
		cfg.PHPPath = detectPHPPath()
		cfg.DefaultPHPVersion = ""
		return cfg, path, err
	}

	if cfg.PHPPath == "" {
		cfg.PHPPath = detectPHPPath()
	}

	if cfg.PHPVersions == nil {
		cfg.PHPVersions = make(map[string]string)
	}

	return cfg, path, nil
}

// scanProjects lists projects that match Laravel or PHP built-in server criteria
func scanProjects(projectsDir string, phpVersions map[string]string, defaultVersion string) ([]Project, error) {
	files, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, err
	}

	var projs []Project
	startPort := 8000

	for _, file := range files {
		if file.IsDir() {
			dirPath := filepath.Join(projectsDir, file.Name())

			// Determine PHP version for this project
			// Priority: config map > .php-version file > default
			phpVer := ""
			if v, ok := phpVersions[file.Name()]; ok && v != "" {
				phpVer = v
			} else if v := detectProjectPHPVersion(dirPath); v != "" {
				phpVer = v
			} else {
				phpVer = defaultVersion
			}

			// 1. Laravel: has artisan in root
			artisanPath := filepath.Join(dirPath, "artisan")
			if _, err := os.Stat(artisanPath); err == nil {
				projs = append(projs, Project{
					Name:       file.Name(),
					Port:       startPort,
					Status:     "OFF",
					Type:       TypeLaravel,
					Path:       dirPath,
					PHPVersion: phpVer,
				})
				startPort++
				continue
			}

			// 2. PHP Built-in (Public index): has public/index.php
			publicIndexPath := filepath.Join(dirPath, "public/index.php")
			if _, err := os.Stat(publicIndexPath); err == nil {
				projs = append(projs, Project{
					Name:       file.Name(),
					Port:       startPort,
					Status:     "OFF",
					Type:       TypePHP,
					Path:       dirPath,
					PHPVersion: phpVer,
				})
				startPort++
				continue
			}

			// 3. PHP Built-in (Root index): has index.php in root
			indexPath := filepath.Join(dirPath, "index.php")
			if _, err := os.Stat(indexPath); err == nil {
				projs = append(projs, Project{
					Name:       file.Name(),
					Port:       startPort,
					Status:     "OFF",
					Type:       TypePHP,
					Path:       dirPath,
					PHPVersion: phpVer,
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

func (m *model) saveConfig() error {
	config := Config{
		ProjectsDir:       m.projectsDir,
		PHPPath:           m.phpPath,
		PHPVersions:       m.phpVersions,
		DefaultPHPVersion: m.defaultPHPVersion,
	}
	configBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.configPath, configBytes, 0644)
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
				ProjectsDir:       val,
				PHPPath:           m.phpPath,
				PHPVersions:       m.phpVersions,
				DefaultPHPVersion: m.defaultPHPVersion,
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

				projs, err := scanProjects(expanded, m.phpVersions, m.defaultPHPVersion)
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

	// PHP Version Picker logic
	if m.state == StatePHPVersionPicker {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.state = StateMain
				return m, nil

			case "up", "k":
				if m.pickerCursor > 0 {
					m.pickerCursor--
				}

			case "down", "j":
				if m.pickerCursor < len(m.availableVersions) {
					m.pickerCursor++
				}

			case "enter":
				p := &m.projects[m.pickerProjectIdx]
				// Stop server if running before changing version
				if p.Status == "RUNNING" && p.Cmd != nil && p.Cmd.Process != nil {
					_ = syscall.Kill(-p.Cmd.Process.Pid, syscall.SIGKILL)
					p.Status = "OFF"
					p.Cmd = nil
				}

				if m.pickerCursor < len(m.availableVersions) {
					// Selected a specific version
					p.PHPVersion = m.availableVersions[m.pickerCursor]
					m.phpVersions[p.Name] = p.PHPVersion
				} else {
					// Selected "default" option
					p.PHPVersion = ""
					delete(m.phpVersions, p.Name)
				}
				saveNginxMap(m.projects)
				m.saveConfig()
				m.state = StateMain
				return m, nil
			}
		}
		return m, nil
	}

	// Set Default PHP Version Picker logic
	if m.state == StateSetDefaultPicker {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.state = StateMain
				return m, nil

			case "up", "k":
				if m.pickerCursor > 0 {
					m.pickerCursor--
				}

			case "down", "j":
				if m.pickerCursor < len(m.availableVersions)-1 {
					m.pickerCursor++
				}

			case "enter":
				selectedVer := m.availableVersions[m.pickerCursor]
				home, _ := os.UserHomeDir()
				src := filepath.Join(home, ".config/herd-lite/bin/php"+selectedVer)
				dst := filepath.Join(home, ".config/herd-lite/bin/php")

				// Stop all running servers first (binary will be replaced)
				for i := range m.projects {
					p := &m.projects[i]
					if p.Status == "RUNNING" && p.Cmd != nil && p.Cmd.Process != nil {
						_ = syscall.Kill(-p.Cmd.Process.Pid, syscall.SIGKILL)
						p.Status = "OFF"
						p.Cmd = nil
					}
				}

				err := copyFile(src, dst)
				if err != nil {
					m.errorMsg = "❌ Gagal menyalin binary PHP: " + err.Error()
					m.state = StateMain
					return m, nil
				}

				// Update detected version
				m.defaultPHPDetected = detectDefaultPHPVersion(m.phpPath)
				m.state = StateMain
				return m, nil
			}
		}
		return m, nil
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

		case "v":
			if len(m.projects) > 0 {
				m.state = StatePHPVersionPicker
				m.pickerProjectIdx = m.cursor
				m.pickerCursor = 0
			}

		case "d":
			if len(m.availableVersions) > 0 {
				m.state = StateSetDefaultPicker
				m.pickerCursor = 0
			}

		case "enter", " ":
			if len(m.projects) == 0 {
				break
			}
			p := &m.projects[m.cursor]
			if p.Status == "OFF" {
				phpBin := resolvePHPBinary(p.PHPVersion, m.phpPath)
				var runCmd *exec.Cmd
				if p.Type == TypeLaravel {
					runCmd = exec.Command(phpBin, "artisan", "serve", "--port="+strconv.Itoa(p.Port))
				} else {
					publicIndexPath := filepath.Join(p.Path, "public/index.php")
					if _, err := os.Stat(publicIndexPath); err == nil {
						runCmd = exec.Command(phpBin, "-S", "127.0.0.1:"+strconv.Itoa(p.Port), "-t", "public")
					} else {
						runCmd = exec.Command(phpBin, "-S", "127.0.0.1:"+strconv.Itoa(p.Port))
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

	// Show available PHP versions if any
	if len(m.availableVersions) > 0 {
		verStr := "PHP versions detected: "
		for i, v := range m.availableVersions {
			if i > 0 {
				verStr += ", "
			}
			verStr += availableVersionStyle.Render(v)
		}
		s += verStr + "\n\n"
	}

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
	s += fmt.Sprintf("Config  : %s\n", m.configPath)

	// Show available PHP versions
	if len(m.availableVersions) > 0 {
		verStr := "Available PHP: "
		for i, v := range m.availableVersions {
			if i > 0 {
				verStr += ", "
			}
			verStr += availableVersionStyle.Render(v)
		}
		s += verStr + "\n"
	}
	// Show detected default PHP version
	if m.defaultPHPDetected != "" {
		s += fmt.Sprintf("Default PHP  : %s %s\n", availableVersionStyle.Render(m.defaultPHPDetected), phpVersionDefaultStyle.Render("(php binary)"))
	}
	s += "\n"

	if len(m.projects) == 0 {
		s += "  (Tidak ada project Laravel atau PHP di folder ini)\n\n"
	} else {
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#475569"))).
			Headers("", "PROJECT NAME", "TYPE", "PHP VER", "PORT", "STATUS", "LOCAL URL")

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

			phpVerStr := "-"
			if p.PHPVersion != "" {
				phpVerStr = p.PHPVersion
			} else if m.defaultPHPVersion != "" {
				phpVerStr = m.defaultPHPVersion + " *"
			}

			t.Row(
				cursorStr,
				p.Name,
				string(p.Type),
				phpVerStr,
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
				if !isSelected && col == 5 { // Status column (shifted due to PHP VER)
					p := m.projects[row]
					if p.Status == "RUNNING" {
						style = statusRunningStyle.Inherit(style)
					} else {
						style = statusOffStyle.Inherit(style)
					}
				}

				// Customize PHP version column
				if !isSelected && col == 3 { // PHP VER column
					p := m.projects[row]
					if p.PHPVersion != "" {
						style = phpVersionStyle.Inherit(style)
					} else {
						style = phpVersionDefaultStyle.Inherit(style)
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
				style = style.Width(16).Align(lipgloss.Left)
			case 3: // PHP VER
				style = style.Width(9).Align(lipgloss.Center)
			case 4: // PORT
				style = style.Width(8).Align(lipgloss.Center)
			case 5: // STATUS
				style = style.Width(12).Align(lipgloss.Center)
			case 6: // LOCAL URL
				style = style.Width(28).Align(lipgloss.Left)
			}

			return style
		})

		s += t.String() + "\n"
	}

	s += "Navigasi: [↑/↓] atau [k/j] | Toggle: [Enter/Spasi] | PHP Ver: [v] | Set Default: [d] | Keluar: [Ctrl+C]\n"
	return s
}

func (m model) View() string {
	if m.state == StateOnboarding {
		return m.viewOnboarding()
	}
	if m.state == StatePHPVersionPicker {
		return m.viewPHPVersionPicker()
	}
	if m.state == StateSetDefaultPicker {
		return m.viewSetDefaultPicker()
	}
	return m.viewMain()
}

func (m model) viewPHPVersionPicker() string {
	p := m.projects[m.pickerProjectIdx]
	s := pickerTitleStyle.Render("🐘 Pilih PHP Version") + "\n"
	s += fmt.Sprintf("Project: %s\n\n", p.Name)

	// Show available versions
	for i, v := range m.availableVersions {
		cursor := "  "
		label := ""
		if m.pickerCursor == i {
			cursor = "▸ "
			label = pickerSelectedStyle.Render(cursor + v)
		} else {
			label = pickerItemStyle.Render(cursor + v)
		}
		s += label + "\n"
	}

	// Default option
	defaultIdx := len(m.availableVersions)
	cursor := "  "
	label := ""
	if m.pickerCursor == defaultIdx {
		cursor = "▸ "
		label = pickerSelectedStyle.Render(cursor + "(default)")
	} else {
		label = pickerItemStyle.Render(cursor + "(default)")
	}
	s += label + "\n"

	s += "\nNavigasi: [↑/↓] atau [k/j] | Pilih: [Enter] | Batal: [Esc]\n"
	return boxStyle.Render(s) + "\n"
}

func (m model) viewSetDefaultPicker() string {
	s := pickerTitleStyle.Render("⚙️  Set Default PHP Version") + "\n"
	s += fmt.Sprintf("Current default: %s\n\n", m.defaultPHPDetected)

	for i, v := range m.availableVersions {
		cursor := "  "
		label := ""
		if m.pickerCursor == i {
			cursor = "▸ "
			label = pickerSelectedStyle.Render(cursor + v)
		} else {
			label = pickerItemStyle.Render(cursor + v)
		}
		s += label + "\n"
	}

	s += "\nNavigasi: [↑/↓] atau [k/j] | Pilih: [Enter] | Batal: [Esc]\n"
	s += warningStyle.Render("⚠ Server yang sedang running akan dihentikan terlebih dahulu.") + "\n"
	return boxStyle.Render(s) + "\n"
}

func main() {
	cfg, configPath, _ := loadConfig()

	var state State
	var projs []Project
	var errMsg string

	// Detect available PHP versions
	availableVersions := detectAvailablePHPVersions()

	// Detect actual version of default PHP binary
	defaultPHPDetected := detectDefaultPHPVersion(cfg.PHPPath)

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
			projs, err = scanProjects(projectsDir, cfg.PHPVersions, cfg.DefaultPHPVersion)
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
		state:              state,
		projects:           projs,
		cursor:             0,
		textInput:          ti,
		errorMsg:           errMsg,
		configPath:         configPath,
		phpPath:            cfg.PHPPath,
		projectsDir:        cfg.ProjectsDir,
		phpVersions:        cfg.PHPVersions,
		defaultPHPVersion:  cfg.DefaultPHPVersion,
		defaultPHPDetected: defaultPHPDetected,
		availableVersions:  availableVersions,
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
