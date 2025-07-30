# Monitor Mutex - Dokumentacja

## Opis

`monitor_mutex.go` to zaawansowany system monitorowania procesów, który automatycznie restartuje aplikacje w przypadku braku aktywności w plikach logów. Monitor działa w oparciu o zasadę "watchdog" - obserwuje pliki logów i restartuje proces jeśli przez określony czas nie pojawiają się nowe wpisy.

## Główne funkcje

### 🔄 Automatyczny restart procesów
- **Monitorowanie plików logów** - wykrywanie nowych wpisów przez rozmiar i czas modyfikacji
- **Inteligentny timeout** - restart po określonym czasie braku aktywności
- **Graceful shutdown** - grzeczne zamykanie procesów (SIGTERM → SIGKILL)

### ⚙️ Elastyczna konfiguracja
- **Tryb pojedynczy** - monitorowanie jednego procesu z parametrami CLI
- **Tryb YAML** - zarządzanie wieloma procesami z pliku konfiguracyjnego
- **Dostosowywalne parametry** - timeout, interwał sprawdzania, ścieżki logów

### 🛡️ Bezpieczeństwo i stabilność
- **Thread-safe** - zabezpieczenie mutex dla operacji wielowątkowych
- **Context handling** - prawidłowe zamykanie przy sygnałach systemowych
- **Error recovery** - kontynuacja działania przy błędach przejściowych

## Sposoby uruchomienia

### 1. Tryb pojedynczy
```bash
./monitor_mutex "komenda" "/ścieżka/do/logów" [timeout] [interwał]
```

### 2. Tryb wieloprocesowy (YAML)
```bash
./monitor_mutex --config konfiguracja.yaml
```

## Przykłady użycia

### Podstawowe monitorowanie
```bash
# Monitoruj aplikację Python
./monitor_mutex "python3 app.py > /tmp/app.log 2>&1" "/tmp/app.log"

# Monitoruj serwer z niestandardowym timeoutem
./monitor_mutex "java -jar server.jar" "/var/log/server.log" 120 10

# Monitoruj skrypt z przekierowaniem logów
./monitor_mutex "bash backup.sh >> /var/log/backup.log" "/var/log/backup.log" 300 30
```

### Konfiguracja YAML
```yaml
# monitor_config.yaml
processes:
  - name: "WebServer"
    command: "python3 -m http.server 8080"
    log_file: "/tmp/webserver.log"
    timeout: 60
    interval: 5

  - name: "Database"
    command: "mysqld --defaults-file=/etc/mysql/my.cnf"
    log_file: "/var/log/mysql/error.log"
    timeout: 120
    interval: 10

  - name: "Worker"
    command: "python3 worker.py >> /tmp/worker.log 2>&1"
    log_file: "/tmp/worker.log"
    timeout: 180
    interval: 15
```

### Uruchomienie z konfiguracją
```bash
# Kompilacja
go build monitor_mutex.go

# Uruchomienie
./monitor_mutex --config monitor_config.yaml
```

## Parametry konfiguracji

### Parametry główne

| Parametr | Opis | Domyślna wartość | Zakres |
|----------|------|------------------|---------|
| `name` | Nazwa procesu (tylko YAML) | - | string |
| `command` | Komenda do uruchomienia | - | string |
| `log_file` | Ścieżka do pliku logów | - | string |
| `timeout` | Timeout w sekundach | 60 | 1-3600 |
| `interval` | Interwał sprawdzania w sekundach | 5 | 1-300 |

### Szczegółowy opis parametrów

#### `command`
Pełna komenda do uruchomienia procesu. Może zawierać:
- Argumenty linii komend
- Przekierowania wyjścia (`>`, `>>`, `2>&1`)
- Pipe'y i łączenie komend (`|`, `&&`)

```bash
# Przykłady poprawnych komend
"python3 app.py --port 8080"
"java -Xmx512m -jar app.jar > app.log 2>&1"
"node server.js | tee /tmp/node.log"
```

#### `log_file`
Ścieżka do pliku, w którym proces zapisuje logi. Monitor:
- Tworzy plik jeśli nie istnieje
- Tworzy katalogi nadrzędne jeśli potrzeba
- Monitoruje zmiany rozmiaru i czasu modyfikacji

#### `timeout`
Czas w sekundach, po którym proces zostanie zrestartowany jeśli nie pojawią się nowe logi:
- **Krótkie (10-30s)** - dla szybkich aplikacji web
- **Średnie (60-120s)** - dla standardowych aplikacji
- **Długie (300s+)** - dla procesów batch/backup

#### `interval`
Częstotliwość sprawdzania stanu procesu:
- **Częste (1-5s)** - dla krytycznych aplikacji
- **Standardowe (5-15s)** - dla większości przypadków
- **Rzadkie (30s+)** - dla procesów o niskim priorytecie

## Algorytm monitorowania

### 1. Inicjalizacja
```
Monitor tworzy kontekst i konfiguruje parametry
↓
Waliduje ścieżki i tworzy katalogi dla logów
↓
Uruchamia pierwszy proces
↓
Zapisuje początkowy stan pliku logów
```

### 2. Główna pętla monitorowania
```
Timer (co `interval` sekund)
↓
Sprawdź czy proces żyje
├─ NIE → Restart procesu
└─ TAK → Sprawdź aktywność logów
           ├─ Nowe logi → Kontynuuj
           ├─ Brak zmian < timeout → Kontynuuj
           └─ Brak zmian ≥ timeout → Restart procesu
```

### 3. Wykrywanie aktywności logów
Monitor sprawdza:
1. **Rozmiar pliku** - czy plik urósł (nowe wpisy)
2. **Czas modyfikacji** - czy plik został zmieniony
3. **Czas od ostatniej zmiany** - czy przekroczono timeout

```go
// Pseudokod logiki
if fileSize > lastSize {
    // Nowe logi - aktualizuj stan
    return aktywny
} else if modTime > lastModTime {
    // Plik przepisany - aktualizuj stan  
    return aktywny
} else if time.Since(lastChange) > timeout {
    // Timeout - restart potrzebny
    return nieaktywny
}
```

## Zarządzanie procesami

### Uruchamianie procesu
```go
// Proces uruchamiany przez shell z kontekstem
cmd := exec.CommandContext(ctx, "sh", "-c", command)
err := cmd.Start()
```

### Zatrzymywanie procesu
1. **SIGTERM** - grzeczne zamknięcie (5s timeout)
2. **SIGKILL** - wymuszone zamknięcie (2s timeout)
3. **Cleanup** - zwolnienie zasobów

```bash
# Sekwencja sygnałów
SIGTERM → czekaj 5s → SIGKILL → czekaj 2s → cleanup
```

### Sprawdzanie stanu
```go
// Test czy proces żyje (sygnał 0)
err := process.Signal(syscall.Signal(0))
if err != nil {
    // Proces nie istnieje
}
```

## Obsługa błędów

### Błędy krytyczne (zakończenie)
- Błąd walidacji ścieżek
- Brak uprawnień do utworzenia plików
- Nieprawidłowa składnia YAML

### Błędy odzyskiwalne (kontynuacja)
- Przejściowe problemy z plikami
- Błędy uruchamiania procesu
- Problemy z sygnałami

### Przykładowy log błędów
```
2024-01-15 14:30:15 ERROR: Nie można odczytać pliku logów: permission denied
2024-01-15 14:30:16 INFO:  Ponawiam sprawdzenie za 5 sekund...
2024-01-15 14:30:20 INFO:  Błąd restartu: exit status 1
2024-01-15 14:30:25 INFO:  Proces zrestartowany pomyślnie
```

## Integracja z systemem

### Systemd Service
```ini
# /etc/systemd/system/monitor.service
[Unit]
Description=Process Monitor
After=network.target

[Service]
Type=simple
User=monitor
WorkingDirectory=/opt/monitor
ExecStart=/opt/monitor/monitor_mutex --config /etc/monitor/config.yaml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

### Logrotate
```bash
# /etc/logrotate.d/monitor
/var/log/monitor/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    postrotate
        systemctl reload monitor.service
    endscript
}
```

### Docker
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY monitor_mutex.go .
RUN go mod init monitor && \
    go get gopkg.in/yaml.v2 && \
    go build -o monitor monitor_mutex.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/monitor .
COPY config.yaml .
CMD ["./monitor", "--config", "config.yaml"]
```

## Monitorowanie i metryki

### Logi systemowe
Monitor wypisuje szczegółowe informacje o:
- Uruchamianiu/zatrzymywaniu procesów
- Stanie plików logów
- Czasach restartów
- Błędach i ostrzeżeniach

### Przykładowy output
```
=== Monitor Procesów ===
Plik logów: /tmp/app.log
Timeout: 1m0s
Interwał sprawdzania: 5s
--------------------------------------------------
Uruchamianie: python3 app.py
Proces uruchomiony z PID: 12345
Początkowy stan logów: rozmiar 0 bajtów
Nowe logi: rozmiar 0 -> 156 bajtów (+156)
Nowe logi: rozmiar 156 -> 312 bajtów (+156)
Oczekiwanie na zmiany w logach... (30s/1m0s)
TIMEOUT! Brak zmian w logach przez 1m5s (limit: 1m0s)
Restartowanie procesu - powód: brak aktywności w logach
Zatrzymywanie procesu PID: 12345
Proces zakończony poprawnie
Uruchamianie: python3 app.py
Proces uruchomiony z PID: 12350
Proces zrestartowany pomyślnie
```

### Metryki do monitorowania
- Liczba restartów na godzinę
- Średni czas działania procesu
- Częstotliwość błędów
- Rozmiar plików logów

## Mechanizm zabijania procesów

### 🛡️ Graceful Shutdown - grzeczne zamykanie

Monitor implementuje inteligentny mechanizm zamykania procesów oparty na dwuetapowej strategii:

#### 1. Faza grzecznego zamknięcia (SIGTERM)
```
SIGTERM → oczekiwanie 5 sekund → sprawdzenie stanu
```

- **Sygnał SIGTERM** - standardowy sygnał zamknięcia systemu Unix/Linux
- **Timeout 5 sekund** - czas na grzeczne zakończenie operacji
- **Proces może**:
  - Zapisać dane do plików
  - Zamknąć połączenia sieciowe  
  - Wyczyścić zasoby
  - Zakończyć się normalnie

#### 2. Faza wymuszonego zamknięcia (SIGKILL)
```
SIGKILL → oczekiwanie 2 sekundy → cleanup
```

- **Sygnał SIGKILL** - natychmiastowe zabicie procesu (nie może być zignorowany)
- **Timeout 2 sekundy** - czas na cleanup systemu
- **Proces zostaje**:
  - Natychmiast zakończony przez kernel
  - Zwolnione zasoby systemowe
  - Usunięty z listy procesów

### 🔍 Szczegółowy przepływ algorytmu

```go
func killProcessUnsafe() {
    // 1. Sprawdź czy proces istnieje
    if process == nil || process.Process == nil {
        return "BRAK_PROCESU"
    }

    pid := process.Process.Pid
    log("Zatrzymywanie procesu PID: %d", pid)

    // 2. Wyślij SIGTERM (15)
    err := process.Signal(syscall.SIGTERM)
    if err != nil {
        return "BŁĄD_SIGTERM"
    }

    // 3. Uruchom goroutine monitorującą
    done := make(chan error, 1)
    go func() {
        done <- process.Wait() // Czeka na zakończenie
    }()

    // 4. Wyścig czasowy - 5 sekund na grzeczne zamknięcie
    select {
    case err := <-done:
        // ✅ SUKCES - proces się zamknął
        if err != nil {
            log("Proces zakończony z błędem: %v", err)
        } else {
            log("Proces zakończony poprawnie")
        }
        return "ZAMKNIĘTY_GRZECZNIE"

    case <-time.After(5 * time.Second):
        // ⏰ TIMEOUT - wymuszenie zamknięcia
        log("Wymuszanie zakończenia procesu (SIGKILL)...")
        
        // 5. Wyślij SIGKILL (9)
        if process.Process != nil {
            process.Process.Kill()
            
            // 6. Drugi wyścig czasowy - 2 sekundy na cleanup
            select {
            case <-done:
                log("Proces zakończony wymuszenie")
                return "ZAMKNIĘTY_WYMUSZENIE"
            case <-time.After(2 * time.Second):
                log("Proces może nie zostać prawidłowo zamknięty")
                return "MOŻE_ZOMBIE"
            }
        }
    }
}
```

### ⚡ Przykłady scenariuszy zamykania

#### Scenariusz 1: Aplikacja współpracująca
```
14:30:15 Zatrzymywanie procesu PID: 12345
14:30:15 → SIGTERM wysłany
14:30:17 ← Proces zakończył się normalnie (2s)
14:30:17 ✅ Proces zakończony poprawnie
```

#### Scenariusz 2: Aplikacja wolno zamykająca
```
14:30:15 Zatrzymywanie procesu PID: 12345  
14:30:15 → SIGTERM wysłany
14:30:20 ⏰ Timeout po 5 sekundach
14:30:20 → SIGKILL wysłany
14:30:20 ← Proces zabity natychmiast
14:30:20 ✅ Proces zakończony wymuszenie
```

#### Scenariusz 3: Proces zawieszony
```
14:30:15 Zatrzymywanie procesu PID: 12345
14:30:15 → SIGTERM wysłany
14:30:20 ⏰ Timeout po 5 sekundach
14:30:20 → SIGKILL wysłany  
14:30:22 ⏰ Timeout po 2 sekundach
14:30:22 ⚠️ Proces może nie zostać prawidłowo zamknięty
```

### 🔒 Thread Safety i synchronizacja

#### Mutex Protection
```go
type Monitor struct {
    // ...existing fields...
    mutex sync.RWMutex // Ochrona dostępu do procesu
}

// Bezpieczne publiczne API
func (m *Monitor) killProcess() {
    m.mutex.Lock()         // Ekskluzywny dostęp
    defer m.mutex.Unlock()
    m.killProcessUnsafe()  // Rzeczywista operacja
}

// Sprawdzanie stanu (współdzielony dostęp)
func (m *Monitor) isProcessRunning() bool {
    m.mutex.RLock()        // Współdzielony dostęp do odczytu
    defer m.mutex.RUnlock()
    
    return m.process != nil && processExists()
}
```

#### Goroutine Management
Monitor używa goroutines do nieblokującego oczekiwania:

```go
// Monitoring procesu w tle
go func() {
    done <- process.Wait()  // Czeka na zakończenie procesu
}()

// Główny wątek może robić inne rzeczy
select {
case result := <-done:     // Proces się zakończył
    handleResult(result)
case <-timeout:            // Upłynął timeout
    forceKill()
}
```

### 🎯 Optymalizacje i edge cases

#### Wykrywanie zombie processes
```go
// Sprawdzenie czy proces nadal istnieje
err := process.Signal(syscall.Signal(0))  // Sygnał "sprawdzający"
if err != nil {
    // errno ESRCH = "No such process"
    return false  // Proces nie istnieje
}
```

#### Context cancellation
```go
// Przerwanie przy shutdown aplikacji
select {
case <-m.ctx.Done():      // Context został anulowany
    m.killProcess()       // Wyczyść zasoby
    return
case <-ticker.C:          // Normalna operacja
    // monitoring logic
}
```

#### Process group handling
```go
// Dla skomplikowanych komend (pipe, &&, ||)
cmd := exec.CommandContext(ctx, "sh", "-c", command)
cmd.SysProcAttr = &syscall.SysProcAttr{
    Setpgid: true,  // Utwórz nową grupę procesów
}

// Zabij całą grupę procesów
syscall.Kill(-pid, syscall.SIGTERM)  // Minus oznacza grupę
```

### 📊 Metryki i monitoring zabijania

#### Typy zakończeń procesów
- **GRACEFUL** - proces zakończył się po SIGTERM (0-5s)
- **FORCED** - proces zabity przez SIGKILL (5-7s)  
- **ZOMBIE** - proces może być w stanie zombie (>7s)
- **ERROR** - błąd podczas wysyłania sygnałów

#### Statystyki w logach
```
=== Statystyki zabijania procesów ===
Grzeczne zamknięcia: 45 (78%)
Wymuszone zamknięcia: 12 (21%) 
Procesy zombie: 1 (1%)
Średni czas grzecznego zamknięcia: 2.3s
Średni czas wymuszonego zamknięcia: 6.8s
```

### ⚠️ Problemy i rozwiązania

#### Problem: Proces ignoruje SIGTERM
**Przyczyna**: Aplikacja przechwytuje sygnał ale nie kończy działania
```go
// Aplikacja może robić to:
signal.Ignore(syscall.SIGTERM)
// lub
signal.Notify(c, syscall.SIGTERM)
for sig := range c {
    log.Printf("Ignoruję sygnał: %v", sig)
    // Ale nie wywołuje os.Exit()
}
```

**Rozwiązanie**: SIGKILL nie może być zignorowany
```go
// Monitor automatycznie użyje SIGKILL po timeout
case <-time.After(5 * time.Second):
    process.Kill()  // SIGKILL - bezwzględne
```
