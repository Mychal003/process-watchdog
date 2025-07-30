# Monitor Mutex - Dokumentacja

## Opis

`monitor_mutex.go` to system monitorowania procesów, który automatycznie restartuje aplikacje w przypadku braku aktywności w plikach logów. Monitor działa w oparciu o zasadę "watchdog" - obserwuje pliki logów i restartuje proces jeśli przez określony czas nie pojawiają się nowe wpisy.

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
- **System retry** - odporność na przejściowe błędy

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

## System prób i odporność na błędy

### 🔄 Mechanizm retry (ponawiania prób)

Monitor nie zabija procesu przy pierwszym problemie - implementuje inteligentny system prób:

#### Parametry systemu prób
```go
maxRetries: 3     // Maksymalnie 3 próby restartu
retryCount: 0     // Bieżący licznik prób
```

#### Scenariusze restart

**Scenariusz 1: Problem przejściowy**
```
1. Timeout logów → Restart (próba 1/3) → Proces działa
2. Reset licznika prób po 10 stabilnych iteracjach (~50s)
3. Kolejny problem → Znów 3 próby dostępne
```

**Scenariusz 2: Poważny problem**
```
1. Timeout logów → Restart (próba 1/3) → Proces pada
2. Proces nie żyje → Restart (próba 2/3) + 5s czekania → Proces pada  
3. Proces nie żyje → Restart (próba 3/3) + 10s czekania → Proces pada
4. ❌ KRYTYCZNY BŁĄD → Monitor kończy działanie
```

#### Reset licznika prób
```go
// Automatyczny reset przy sukcesie
if newLogsDetected || fileChanged {
    m.retryCount = 0  // Reset przy aktywności
}

// Reset po stabilnym działaniu  
if stableIterations >= 10 {  // ~50 sekund stabilności
    m.resetRetries()
    stableIterations = 0
}
```

#### Progresywne opóźnienia
```go
// Zwiększanie czasu oczekiwania przy kolejnych próbach
waitTime := time.Duration(m.retryCount) * time.Second * 5
// próba 1: 0s, próba 2: 5s, próba 3: 10s
```

### 📊 Przykładowy log systemu prób

```
14:30:00 TIMEOUT! Brak zmian w logach przez 1m5s (limit: 1m0s)
14:30:00 Restartowanie procesu - powód: brak aktywności w logach
14:30:01 ✅ Proces zrestartowany pomyślnie
14:30:15 Proces przestał działać  
14:30:15 Restartowanie procesu - powód: proces przestał działać (próba 2/3)
14:30:15 Oczekiwanie 5s przed kolejną próbą...
14:30:21 ✅ Proces zrestartowany pomyślnie (próba 2/3)
14:31:25 🔄 Reset licznika prób (było: 1)  # Po stabilnym działaniu
```

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
├─ NIE → Restart procesu (z retry)
└─ TAK → Sprawdź aktywność logów
           ├─ Nowe logi → Kontynuuj + reset licznika prób
           ├─ Brak zmian < timeout → Kontynuuj
           └─ Brak zmian ≥ timeout → Restart procesu (z retry)
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
    m.retryCount = 0  // Reset licznika prób
    return aktywny
} else if modTime > lastModTime {
    // Plik przepisany - aktualizuj stan  
    m.retryCount = 0  // Reset licznika prób
    return aktywny
} else if time.Since(lastChange) > timeout {
    // Timeout - restart potrzebny
    return nieaktywny
}
```

## Mechanizm zabijania procesów

### 🛡️ Graceful Shutdown - grzeczne zamykanie

Monitor implementuje dwuetapową strategię zamykania procesów:

#### 1. Faza grzecznego zamknięcia (SIGTERM)
```
SIGTERM → oczekiwanie 5 sekund → sprawdzenie stanu
```

- **Sygnał SIGTERM** - standardowy sygnał zamknięcia systemu Unix/Linux
- **Timeout 5 sekund** - czas na grzeczne zakończenie operacji
- **Proces może**: zapisać dane, zamknąć połączenia, wyczyścić zasoby

#### 2. Faza wymuszonego zamknięcia (SIGKILL)
```
SIGKILL → oczekiwanie 2 sekundy → cleanup
```

- **Sygnał SIGKILL** - natychmiastowe zabicie procesu (nie może być zignorowany)
- **Timeout 2 sekundy** - czas na cleanup systemu

### 🔍 Szczegółowy przepływ algorytmu

```go
func killProcessUnsafe() {
    // 1. Sprawdź czy proces istnieje
    if process == nil || process.Process == nil {
        return
    }

    // 2. Wyślij SIGTERM (15)
    err := process.Signal(syscall.SIGTERM)
    
    // 3. Uruchom goroutine monitorującą
    done := make(chan error, 1)
    go func() {
        done <- process.Wait() // Czeka na zakończenie
    }()

    // 4. Wyścig czasowy - 5 sekund na grzeczne zamknięcie
    select {
    case err := <-done:
        // ✅ SUKCES - proces się zamknął
        log("Proces zakończony poprawnie")
        
    case <-time.After(5 * time.Second):
        // ⏰ TIMEOUT - wymuszenie zamknięcia
        log("Wymuszanie zakończenia procesu (SIGKILL)...")
        
        // 5. Wyślij SIGKILL (9)
        process.Process.Kill()
        
        // 6. Drugi wyścig czasowy - 2 sekundy na cleanup
        select {
        case <-done:
            log("Proces zakończony wymuszenie")
        case <-time.After(2 * time.Second):
            log("Proces może nie zostać prawidłowo zamknięty")
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

## Zarządzanie procesami

### Uruchamianie procesu
```go
// Proces uruchamiany przez shell z kontekstem
cmd := exec.CommandContext(ctx, "sh", "-c", command)
err := cmd.Start()
```

### Sprawdzanie stanu procesu
```go
// Test czy proces żyje (sygnał 0)
err := process.Signal(syscall.Signal(0))
if err != nil {
    // Proces nie istnieje
}
```

### Thread Safety
```go
type Monitor struct {
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

## Obsługa błędów

### Błędy krytyczne (zakończenie monitora)
- Przekroczenie maksymalnej liczby prób restartu (3)
- Błąd walidacji ścieżek
- Brak uprawnień do utworzenia plików
- Nieprawidłowa składnia YAML

### Błędy odzyskiwalne (kontynuacja)
- Przejściowe problemy z plikami logów
- Błędy uruchamiania procesu (retry)
- Problemy z sygnałami

### Przykładowy log błędów
```
14:30:15 TIMEOUT! Brak zmian w logach przez 1m5s (limit: 1m0s)
14:30:15 Restartowanie procesu - powód: brak aktywności w logach
14:30:16 Błąd restartu: exit status 1 (próba 1/3)
14:30:16 Oczekiwanie 5s przed kolejną próbą...
14:30:21 ✅ Proces zrestartowany pomyślnie (próba 2/3)
14:30:45 🔄 Reset licznika prób (było: 1)
```

## Przykładowy output monitora

```
=== Monitor Procesów ===
Plik logów: /tmp/app.log
Timeout: 1m0s
Interwał sprawdzania: 5s
Maksymalna liczba prób restartu: 3
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
✅ Proces zrestartowany pomyślnie
🔄 Reset licznika prób (było: 0)
```

## Instalacja i konfiguracja

### Kompilacja
```bash
# Inicjalizacja modułu Go
go mod init monitor

# Pobranie zależności YAML (opcjonalne)
go get gopkg.in/yaml.v2

# Kompilacja
go build monitor_mutex.go
```

### Uruchomienie jako usługa systemd
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

### Zarządzanie usługą
```bash
# Włączenie i uruchomienie
sudo systemctl enable monitor.service
sudo systemctl start monitor.service

# Sprawdzenie statusu
sudo systemctl status monitor.service

# Podgląd logów
sudo journalctl -u monitor.service
