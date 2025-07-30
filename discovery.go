package main

import (
    "bufio"
    "fmt"
    "os"
    "os/exec"
    "regexp"
    "strconv"
    "strings"
    "time"
)

// Kandydat do monitorowania
type ProcessCandidate struct {
    Name        string
    PID         string
    Command     string
    LogFile     string
    Port        string
    User        string
    CPUUsage    string
    MemoryUsage string
}

// Konfiguracja procesu do monitorowania
type ProcessConfig struct {
    Name     string
    Command  string
    LogFile  string
    Timeout  int
    Interval int
}

// Główna funkcja discovery
func discoverProcesses() []ProcessCandidate {
    fmt.Println("🔍 Skanowanie systemu w poszukiwaniu procesów...")
    
    var candidates []ProcessCandidate
    
    // 1. Procesy z otwartymi plikami logów
    logCandidates := findProcessesWithLogs()
    fmt.Printf("   Znaleziono %d procesów z plikami logów\n", len(logCandidates))
    candidates = append(candidates, logCandidates...)
    
    // 2. Procesy nasłuchujące na portach
    portCandidates := findListeningProcesses()
    fmt.Printf("   Znaleziono %d procesów nasłuchujących\n", len(portCandidates))
    candidates = append(candidates, portCandidates...)
    
    // 3. Długo działające procesy
    longCandidates := findLongRunningProcesses()
    fmt.Printf("   Znaleziono %d długo działających procesów\n", len(longCandidates))
    candidates = append(candidates, longCandidates...)
    
    // Usuń duplikaty
    candidates = removeDuplicates(candidates)
    
    fmt.Printf("   Łącznie: %d unikalnych kandydatów\n\n", len(candidates))
    return candidates
}

// Znajdź procesy z otwartymi plikami logów
func findProcessesWithLogs() []ProcessCandidate {
    // Sprawdź czy lsof jest dostępne
    if _, err := exec.LookPath("lsof"); err != nil {
        return findLogProcessesFallback()
    }
    
    cmd := exec.Command("lsof", "+c", "0", "-n")
    output, err := cmd.Output()
    if err != nil {
        return findLogProcessesFallback()
    }
    
    var candidates []ProcessCandidate
    lines := strings.Split(string(output), "\n")
    
    logPattern := regexp.MustCompile(`\.log$|/var/log/|/tmp/.*\.log|\.out$`)
    seen := make(map[string]bool) // Zapobiegaj duplikatom
    
    for _, line := range lines {
        if logPattern.MatchString(line) {
            fields := strings.Fields(line)
            if len(fields) >= 9 {
                pid := fields[1]
                logFile := fields[8]
                
                // Sprawdź czy już mamy ten proces
                key := pid + ":" + logFile
                if seen[key] {
                    continue
                }
                seen[key] = true
                
                // Pobierz rzeczywistą komendę z PID
                realCommand := getCommandFromPID(pid)
                if realCommand == "" {
                    continue // Pomiń procesy bez komendy
                }
                
                // Sprawdź czy komenda nie jest ścieżką do pliku logów
                if realCommand == logFile || strings.HasSuffix(realCommand, logFile) {
                    continue
                }
                
                candidate := ProcessCandidate{
                    Name:    fields[0],
                    PID:     pid,
                    User:    fields[2],
                    LogFile: logFile,
                    Command: realCommand,
                }
                candidates = append(candidates, candidate)
            }
        }
    }
    
    return candidates
}

// Fallback dla systemów bez lsof
func findLogProcessesFallback() []ProcessCandidate {
    var candidates []ProcessCandidate
    
    fmt.Println("   ⚠️  lsof niedostępne, używam metody fallback...")
    
    // Sprawdź popularne lokalizacje logów
    logDirs := []string{"/var/log", "/tmp"}
    
    for _, dir := range logDirs {
        if files, err := findLogFiles(dir); err == nil {
            for _, file := range files {
                if len(file) > 0 {
                    candidates = append(candidates, ProcessCandidate{
                        Name:    "manual-check",
                        LogFile: file,
                        Command: fmt.Sprintf("# Proces korzystający z %s - wymagana ręczna konfiguracja", file),
                    })
                }
            }
        }
    }
    
    return candidates
}

// Znajdź pliki logów w katalogu
func findLogFiles(dir string) ([]string, error) {
    cmd := exec.Command("find", dir, "-name", "*.log", "-type", "f", "2>/dev/null")
    output, err := cmd.Output()
    if err != nil {
        return nil, err
    }
    
    files := strings.Split(strings.TrimSpace(string(output)), "\n")
    return files, nil
}

// Znajdź procesy nasłuchujące na portach
func findListeningProcesses() []ProcessCandidate {
    // Próbuj ss (nowszy) lub netstat (starszy)
    cmd := exec.Command("ss", "-tlnp")
    output, err := cmd.Output()
    if err != nil {
        // Fallback do netstat
        cmd = exec.Command("netstat", "-tlnp")
        output, err = cmd.Output()
        if err != nil {
            return nil
        }
    }
    
    var candidates []ProcessCandidate
    lines := strings.Split(string(output), "\n")
    
    for _, line := range lines {
        if strings.Contains(line, "LISTEN") {
            fields := strings.Fields(line)
            if len(fields) >= 6 {
                // Parsuj port
                var port string
                if addr := fields[3]; strings.Contains(addr, ":") {
                    parts := strings.Split(addr, ":")
                    port = parts[len(parts)-1]
                }
                
                // Parsuj proces (różne formaty w ss vs netstat)
                var processInfo string
                for _, field := range fields {
                    if strings.Contains(field, "/") {
                        processInfo = field
                        break
                    }
                }
                
                if processInfo != "" && port != "" {
                    parts := strings.Split(processInfo, "/")
                    if len(parts) >= 2 {
                        candidate := ProcessCandidate{
                            Name: parts[1],
                            PID:  parts[0],
                            Port: port,
                        }
                        candidates = append(candidates, candidate)
                    }
                }
            }
        }
    }
    
    return candidates
}

// Znajdź długo działające procesy (starsze niż 1 godzina)
func findLongRunningProcesses() []ProcessCandidate {
    cmd := exec.Command("ps", "aux", "--sort=-etime")
    output, err := cmd.Output()
    if err != nil {
        return nil
    }
    
    var candidates []ProcessCandidate
    lines := strings.Split(string(output), "\n")
    
    // Pomiń nagłówek
    if len(lines) > 1 {
        lines = lines[1:]
    }
    
    for i, line := range lines {
        if i > 20 { // Ograniczenie do pierwszych 20 procesów
            break
        }
        
        fields := strings.Fields(line)
        if len(fields) >= 11 {
            // Sprawdź czy proces działa dłużej niż godzinę
            etime := fields[9] // FORMAT: [[DD-]HH:]MM:SS
            if isLongRunning(etime) {
                candidate := ProcessCandidate{
                    Name:        extractProcessName(fields[10:]),
                    PID:         fields[1],
                    User:        fields[0],
                    CPUUsage:    fields[2],
                    MemoryUsage: fields[3],
                    Command:     strings.Join(fields[10:], " "),
                }
                candidates = append(candidates, candidate)
            }
        }
    }
    
    return candidates
}

// Sprawdź czy proces działa długo
func isLongRunning(etime string) bool {
    // Format: [[DD-]HH:]MM:SS
    if strings.Contains(etime, "-") {
        return true // Dni = na pewno długo
    }
    if strings.Count(etime, ":") == 2 {
        return true // HH:MM:SS = więcej niż godzina
    }
    return false
}

// Wyciągnij nazwę procesu z komendy
func extractProcessName(cmdFields []string) string {
    if len(cmdFields) == 0 {
        return "unknown"
    }
    
    cmd := cmdFields[0]
    
    // Usuń ścieżkę
    if strings.Contains(cmd, "/") {
        parts := strings.Split(cmd, "/")
        cmd = parts[len(parts)-1]
    }
    
    // Usuń argumenty
    if strings.Contains(cmd, " ") {
        cmd = strings.Split(cmd, " ")[0]
    }
    
    return cmd
}

// Usuń duplikaty na podstawie nazwy
func removeDuplicates(candidates []ProcessCandidate) []ProcessCandidate {
    seen := make(map[string]bool)
    var unique []ProcessCandidate
    
    for _, candidate := range candidates {
        // Użyj kombinacji nazwy i komendy jako klucza
        key := candidate.Name + ":" + candidate.Command
        
        // Podstawowa walidacja
        if candidate.Name == "" || 
           candidate.Name == "unknown" || 
           candidate.Command == "" ||
           candidate.Command == candidate.LogFile {
            continue
        }
        
        if !seen[key] {
            seen[key] = true
            unique = append(unique, candidate)
        }
    }
    
    return unique
}

// Interaktywny wybór procesów
func selectProcessesToMonitor() []ProcessCandidate {
    candidates := discoverProcesses()
    
    if len(candidates) == 0 {
        fmt.Println("❌ Nie znaleziono kandydatów do monitorowania")
        fmt.Println("Spróbuj ręcznej konfiguracji lub uruchom więcej procesów")
        return nil
    }
    
    fmt.Println("📋 Znalezione kandydaci do monitorowania:")
    fmt.Println("==========================================")
    
    for i, candidate := range candidates {
        fmt.Printf("[%2d] %-20s", i+1, candidate.Name)
        
        if candidate.PID != "" {
            fmt.Printf(" PID: %-8s", candidate.PID)
        }
        
        if candidate.Port != "" {
            fmt.Printf(" Port: %-6s", candidate.Port)
        }
        
        if candidate.CPUUsage != "" {
            fmt.Printf(" CPU: %-5s%%", candidate.CPUUsage)
        }
        
        fmt.Println()
        
        if candidate.LogFile != "" {
            fmt.Printf("     Log: %s\n", candidate.LogFile)
        }
        
        if candidate.Command != "" {
            cmd := candidate.Command
            if len(cmd) > 60 {
                cmd = cmd[:57] + "..."
            }
            fmt.Printf("     Cmd: %s\n", cmd)
        }
        
        fmt.Println()
    }
    
    // Czytaj wybór użytkownika
    reader := bufio.NewReader(os.Stdin)
    fmt.Print("Wybierz numery procesów do monitorowania (np: 1,3,5 lub 'all' dla wszystkich): ")
    input, _ := reader.ReadString('\n')
    input = strings.TrimSpace(input)
    
    if input == "" {
        return nil
    }
    
    var selected []ProcessCandidate
    
    if input == "all" {
        selected = candidates
    } else {
        for _, numStr := range strings.Split(input, ",") {
            numStr = strings.TrimSpace(numStr)
            if num, err := strconv.Atoi(numStr); err == nil && num > 0 && num <= len(candidates) {
                selected = append(selected, candidates[num-1])
            }
        }
    }
    
    return selected
}



// Lista procesów systemowych do pominięcia
var systemProcesses = []string{
    "systemd",
    "kernel",
    "rsyslogd", 
    "dbus",
    "networkd",
    "resolved",
    "unattended-upgrade",
    "cron",
    "ssh",
    "getty",
    "udev",
    "polkit",
    "avahi",
    "cups",
    "bluetooth",
    "ModemManager",
    "NetworkManager",
    "systemd-logind",
    "systemd-networkd",
    "systemd-resolved",
    "systemd-timesyncd",
    "systemd-udevd",
    "wpa_supplicant",
}

// Sprawdź czy proces jest procesem systemowym
func isSystemProcess(name, command string) bool {
    nameLower := strings.ToLower(name)
    commandLower := strings.ToLower(command)
    
    // Rozszerzona lista procesów systemowych
    systemProcesses := []string{
        "systemd", "kernel", "rsyslogd", "journal", // ❗ DODAJ journal
        "dbus", "networkd", "resolved", "unattended",
        "cron", "ssh", "getty", "udev", "polkit",
        "avahi", "cups", "bluetooth", "ModemManager",
        "NetworkManager", "wpa_supplicant",
    }
    
    // Sprawdź listę znanych procesów systemowych
    for _, sysProc := range systemProcesses {
        if strings.Contains(nameLower, sysProc) || 
           strings.Contains(commandLower, sysProc) {
            return true
        }
    }
    
    // Sprawdź systemowe ścieżki - WZMOCNIONE
    systemPaths := []string{
        "/usr/lib/systemd/", "/lib/systemd/",
        "/usr/sbin/", "/sbin/",
        "/usr/share/unattended-upgrades/", // ❗ DODAJ TĘ ŚCIEŻKĘ
    }
    
    for _, path := range systemPaths {
        if strings.HasPrefix(command, path) {
            return true
        }
    }
    
    // Sprawdź procesy kernela i init
    if strings.HasPrefix(nameLower, "kthread") ||
       strings.HasPrefix(nameLower, "kernel") ||
       strings.HasPrefix(nameLower, "init") ||
       strings.Contains(nameLower, "worker") {
        return true
    }
    
    return false
}

// Sprawdź czy proces może być bezpiecznie zrestartowany
func isSafeToRestart(candidate ProcessCandidate) bool {
    command := strings.ToLower(candidate.Command)
    name := strings.ToLower(candidate.Name)
    
    // 1. NAJPIERW sprawdź czy to niebezpieczny proces
    dangerousProcesses := []string{
        "init", "kernel", "systemd", "dbus", "udev", "network",
        "ssh", "getty", "login", "su", "sudo", "mount", "umount",
        "rsyslog", "journal", "unattended-upgrade", // ❗ DODAJ TE
    }
    
    for _, dangerous := range dangerousProcesses {
        if strings.Contains(name, dangerous) || strings.Contains(command, dangerous) {
            return false // ❌ NIEBEZPIECZNY - odrzuć
        }
    }
    
    // 2. Sprawdź systemowe lokalizacje
    if strings.HasPrefix(candidate.Command, "/usr/lib/systemd/") ||
       strings.HasPrefix(candidate.Command, "/lib/systemd/") ||
       strings.HasPrefix(candidate.Command, "/usr/sbin/") ||
       strings.HasPrefix(candidate.Command, "/sbin/") {
        return false // ❌ SYSTEMOWY - odrzuć
    }
    
    // 3. DOPIERO TERAZ sprawdź czy to proces użytkownika (bezpieczniejszy)
    if candidate.User != "" && candidate.User != "root" && candidate.User != "system" {
        return true // ✅ Proces użytkownika - bezpieczny
    }
    
    // 4. Dla pozostałych procesów root - sprawdź bezpieczne lokalizacje
    if strings.HasPrefix(candidate.Command, "/home/") ||
       strings.HasPrefix(candidate.Command, "/opt/") ||
       strings.HasPrefix(candidate.Command, "/usr/local/") ||
       strings.HasPrefix(candidate.Command, "/tmp/") {
        return true // ✅ Bezpieczna lokalizacja
    }
    
    // 5. Domyślnie odrzuć nieznane procesy root
    return false
}

// ...existing code...

// Sugeruj konfigurację dla wybranych procesów
func suggestConfiguration(candidates []ProcessCandidate) []ProcessConfig {
    var configs []ProcessConfig
    
    fmt.Println("\n🔧 Generowanie konfiguracji...")
    fmt.Println("==============================")
    
    for _, candidate := range candidates {
        // 1. Podstawowa walidacja
        if candidate.Command == "" || candidate.Command == candidate.LogFile {
            fmt.Printf("⚠️  Pominięto %s - brak poprawnej komendy\n", candidate.Name)
            continue
        }
        
        // 2. Sprawdź czy to proces systemowy
        if isSystemProcess(candidate.Name, candidate.Command) {
            fmt.Printf("🚫 Pominięto proces systemowy: %s (%s)\n", 
                      candidate.Name, candidate.Command)
            continue
        }
        
        // 3. Sprawdź czy bezpieczny do restartu
        if !isSafeToRestart(candidate) {
            fmt.Printf("⚠️  Pominięto niebezpieczny proces: %s - może być krytyczny dla systemu\n", 
                      candidate.Name)
            continue
        }
        
        // 4. Tworzenie konfiguracji
        config := ProcessConfig{
            Name:     candidate.Name,
            Timeout:  60,  // Domyślny timeout
            Interval: 5,   // Domyślny interwał
        }
        
        // 5. Ustaw komendę
        config.Command = candidate.Command
        
        // 6. Ustaw plik logów
        if candidate.LogFile != "" {
            config.LogFile = candidate.LogFile
        } else {
            // Sugeruj bezpieczną lokalizację dla logów
            if candidate.User != "" && candidate.User != "root" {
                config.LogFile = fmt.Sprintf("/tmp/%s_%s.log", 
                                           candidate.User, strings.ToLower(candidate.Name))
            } else {
                config.LogFile = fmt.Sprintf("/tmp/%s.log", strings.ToLower(candidate.Name))
            }
        }
        
        // 7. Dostosuj parametry na podstawie typu procesu
        nameLower := strings.ToLower(candidate.Name)
        commandLower := strings.ToLower(candidate.Command)
        
        // Serwery sieciowe - średni timeout
        if candidate.Port != "" || 
           strings.Contains(nameLower, "server") ||
           strings.Contains(commandLower, "listen") ||
           strings.Contains(commandLower, "daemon") {
            config.Timeout = 90
            config.Interval = 10
        }
        
        // Bazy danych - długi timeout
        if strings.Contains(nameLower, "database") ||
           strings.Contains(nameLower, "mysql") ||
           strings.Contains(nameLower, "postgres") ||
           strings.Contains(nameLower, "redis") ||
           strings.Contains(nameLower, "mongo") ||
           strings.Contains(commandLower, "sql") {
            config.Timeout = 180
            config.Interval = 15
        }
        
        // Serwery web - średni timeout
        if strings.Contains(nameLower, "nginx") ||
           strings.Contains(nameLower, "apache") ||
           strings.Contains(nameLower, "httpd") ||
           strings.Contains(nameLower, "tomcat") ||
           strings.Contains(commandLower, "http") {
            config.Timeout = 120
            config.Interval = 10
        }
        
        // Aplikacje Java - długi timeout (powolny start)
        if strings.Contains(commandLower, "java") ||
           strings.Contains(commandLower, ".jar") {
            config.Timeout = 300
            config.Interval = 20
        }
        
        // Skrypty i małe aplikacje - krótki timeout
        if strings.Contains(commandLower, "bash") ||
           strings.Contains(commandLower, "python") ||
           strings.Contains(commandLower, "node") ||
           strings.Contains(commandLower, "ruby") ||
           strings.Contains(commandLower, "php") {
            config.Timeout = 45
            config.Interval = 8
        }
        
        // Procesy w /tmp - bardzo krótki timeout (testy)
        if strings.HasPrefix(config.LogFile, "/tmp/") {
            config.Timeout = 30
            config.Interval = 5
        }
        
        // 8. Walidacja końcowa
        if config.Timeout < config.Interval {
            config.Timeout = config.Interval * 3 // Minimum 3 interwały
        }
        
        configs = append(configs, config)
        
        // 9. Pokaż informacje o dodanym procesie
        status := "✅"
        if candidate.User == "root" {
            status = "⚠️ "
        }
        
        fmt.Printf("%s %s -> %s (timeout: %ds, interval: %ds)\n", 
                   status, config.Name, config.LogFile, config.Timeout, config.Interval)
        
        if candidate.Port != "" {
            fmt.Printf("    🌐 Port: %s\n", candidate.Port)
        }
        if candidate.User != "" {
            fmt.Printf("    👤 Użytkownik: %s\n", candidate.User)
        }
    }
    
    // 10. Podsumowanie
    fmt.Printf("\n📊 Podsumowanie:\n")
    fmt.Printf("   Kandydatów: %d\n", len(candidates))
    fmt.Printf("   Zaakceptowanych: %d\n", len(configs))
    fmt.Printf("   Odrzuconych: %d\n", len(candidates)-len(configs))
    
    if len(configs) == 0 {
        fmt.Println("\n⚠️  Brak procesów spełniających kryteria do monitorowania")
        fmt.Println("💡 Sugestie:")
        fmt.Println("   - Uruchom własne aplikacje (python, node, java)")
        fmt.Println("   - Sprawdź procesy użytkownika (nie root)")
        fmt.Println("   - Utwórz testowe procesy w /tmp/")
        fmt.Println("\n   Przykład testowego procesu:")
        fmt.Println("   nohup bash -c 'while true; do echo $(date) Test app; sleep 10; done > /tmp/test.log' &")
    } else {
        fmt.Println("\n✅ Konfiguracja gotowa do użycia!")
    }
    
    return configs
}
// Pobierz komendę z PID
func getCommandFromPID(pid string) string {
    // Pierwsza próba - ps z pełną komendą
    cmd := exec.Command("ps", "-p", pid, "-o", "cmd", "--no-headers")
    output, err := cmd.Output()
    if err == nil && len(output) > 0 {
        cmdStr := strings.TrimSpace(string(output))
        if cmdStr != "" && cmdStr != "<defunct>" {
            return cmdStr
        }
    }
    
    // Druga próba - /proc/PID/cmdline
    cmd = exec.Command("cat", fmt.Sprintf("/proc/%s/cmdline", pid))
    output, err = cmd.Output()
    if err == nil && len(output) > 0 {
        // Zamień null bytes na spacje i oczyść
        cmdStr := strings.ReplaceAll(string(output), "\x00", " ")
        cmdStr = strings.TrimSpace(cmdStr)
        if cmdStr != "" {
            return cmdStr
        }
    }
    
    // Trzecia próba - /proc/PID/comm (nazwa procesu)
    cmd = exec.Command("cat", fmt.Sprintf("/proc/%s/comm", pid))
    output, err = cmd.Output()
    if err == nil && len(output) > 0 {
        return strings.TrimSpace(string(output))
    }
    
    return ""
}


// Zapisz konfigurację do pliku YAML
func saveConfiguration(configs []ProcessConfig, filename string) error {
    file, err := os.Create(filename)
    if err != nil {
        return fmt.Errorf("nie można utworzyć pliku: %v", err)
    }
    defer file.Close()

    // Nagłówek pliku
    fmt.Fprintf(file, "# Automatycznie wygenerowana konfiguracja monitora\n")
    fmt.Fprintf(file, "# Data: %s\n", time.Now().Format("2006-01-02 15:04:05"))
    fmt.Fprintf(file, "# Użycie: monitor_mutex --config %s\n\n", filename)
    fmt.Fprintf(file, "processes:\n")

    // Dla każdego procesu
    for _, config := range configs {
        fmt.Fprintf(file, "  - name: %q\n", config.Name)
        
        // Bezpieczne cytowanie komendy - używaj pojedynczych cudzysłowów dla zewnętrznych
        // i escape dla wewnętrznych
        safeCommand := strings.ReplaceAll(config.Command, `"`, `\"`)
        fmt.Fprintf(file, "    command: %q\n", safeCommand)
        
        fmt.Fprintf(file, "    log_file: %q\n", config.LogFile)
        fmt.Fprintf(file, "    timeout: %d\n", config.Timeout)
        fmt.Fprintf(file, "    interval: %d\n\n", config.Interval)
    }

    fmt.Fprintf(file, "# Uruchom monitor poleceniem:\n")
    fmt.Fprintf(file, "# ./monitor_mutex --config %s\n", filename)

    return nil
}


// Główna funkcja discovery
func main() {
    fmt.Println("=== MONITOR DISCOVERY ===")
    fmt.Println("Narzędzie do automatycznego wykrywania procesów do monitorowania\n")
    
    selected := selectProcessesToMonitor()
    if len(selected) == 0 {
        fmt.Println("Nie wybrano żadnych procesów. Zakończenie.")
        return
    }
    
    configs := suggestConfiguration(selected)
    
    fmt.Print("\nCzy zapisać konfigurację do pliku? (t/n): ")
    reader := bufio.NewReader(os.Stdin)
    input, _ := reader.ReadString('\n')
    
    if strings.TrimSpace(strings.ToLower(input)) == "t" {
        fmt.Print("Podaj nazwę pliku (enter = monitor_config.yaml): ")
        filename, _ := reader.ReadString('\n')
        filename = strings.TrimSpace(filename)
        
        if filename == "" {
            filename = "monitor_config.yaml"
        }
        
        if !strings.HasSuffix(filename, ".yaml") && !strings.HasSuffix(filename, ".yml") {
            filename += ".yaml"
        }
        
        if err := saveConfiguration(configs, filename); err != nil {
            fmt.Printf("❌ Błąd zapisu: %v\n", err)
        } else {
            fmt.Printf("✅ Konfiguracja zapisana do: %s\n", filename)
            fmt.Printf("\nUruchom monitor poleceniem:\n")
            fmt.Printf("./monitor_mutex --config %s\n", filename)
        }
    }
    
}