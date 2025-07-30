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

## Rozwiązywanie problemów

### Problem: Process nie startuje
```bash
# Sprawdź czy komenda jest poprawna
sh -c "python3 app.py"

# Sprawdź uprawnienia
ls -la /path/to/app.py

# Sprawdź zależności
which python3
```

### Problem: Częste restarty
```yaml
# Zwiększ timeout
timeout: 300  # z 60 na 300 sekund

# Zmniejsz interwał (szybsze wykrywanie aktywności)
interval: 2   # z 5 na 2 sekundy
```

### Problem: Logi nie są wykrywane
```bash
# Sprawdź czy aplikacja faktycznie pisze do pliku
tail -f /tmp/app.log

# Sprawdź uprawnienia do pliku
ls -la /tmp/app.log

# Sprawdź czy plik jest zapisywany
watch "ls -la /tmp/app.log"
```

### Problem: Monitor się zawiesza
```bash
# Sprawdź procesy
ps aux | grep monitor

# Sprawdź sygnały
kill -USR1 <monitor_pid>  # debug info
kill -TERM <monitor_pid>  # graceful shutdown
```

## Zaawansowane przypadki użycia

### 1. Monitoring klastra aplikacji
```yaml
processes:
  - name: "web-1"
    command: "python3 app.py --port 8001"
    log_file: "/var/log/web-1.log"
    timeout: 30
    interval: 5
    
  - name: "web-2" 
    command: "python3 app.py --port 8002"
    log_file: "/var/log/web-2.log"
    timeout: 30
    interval: 5
    
  - name: "worker"
    command: "python3 worker.py"
    log_file: "/var/log/worker.log"
    timeout: 120
    interval: 10
```

### 2. Monitoring z preprocessing
```yaml
processes:
  - name: "data-processor"
    command: "python3 processor.py | tee /tmp/processor.log"
    log_file: "/tmp/processor.log"
    timeout: 600  # 10 minut na batch
    interval: 30
```

### 3. Monitoring z cleanup
```yaml
processes:
  - name: "cleaner"
    command: "bash -c 'while true; do cleanup.sh >> /tmp/clean.log 2>&1; sleep 3600; done'"
    log_file: "/tmp/clean.log" 
    timeout: 7200  # 2 godziny
    interval: 60
```

## Porównanie z innymi rozwiązaniami

### vs Systemd
| Cecha | Monitor Mutex | Systemd |
|-------|---------------|---------|
| Restart na brak logów | ✅ | ❌ |
| Konfiguracja YAML | ✅ | ❌ |
| Lekki footprint | ✅ | ❌ |
| Integracja systemowa | ❌ | ✅ |

### vs Supervisor
| Cecha | Monitor Mutex | Supervisor |
|-------|---------------|------------|
| Monitoring logów | ✅ | ❌ |
| Go dependency | ✅ | ❌ |
| Python dependency | ❌ | ✅ |
| Web interface | ❌ | ✅ |

### vs Docker healthcheck
| Cecha | Monitor Mutex | Docker |
|-------|---------------|---------|
| Log-based restart | ✅ | ❌ |
| Native processes | ✅ | ❌ |
| Container overhead | ❌ | ✅ |
| Orchestration | ❌ | ✅ |

## Wymagania systemowe

### Minimalne wymagania
- **OS**: Linux, macOS, Windows (ograniczone)
- **RAM**: 10MB
- **CPU**: Dowolny (bardzo niskie użycie)
- **Go**: 1.19+ (do kompilacji)

### Zalecane narzędzia
- `ps` - informacje o procesach
- `kill` - wysyłanie sygnałów
- `tail` - debugowanie logów

### Zależności Go
```bash
go mod init monitor
go get gopkg.in/yaml.v2
```

## Rozwój i contribucje

### Planowane funkcje
- [ ] Metryki Prometheus
- [ ] Web dashboard
- [ ] Slack/Discord notyfikacje
- [ ] Rolling restarts
- [ ] Health checks HTTP
- [ ] Log parsing rules

### Struktura kodu
```
monitor_mutex.go
├── Types (Config, Monitor, ProcessConfig)
├── Core Logic (Monitor.Run, checkLogs, startProcess)
├── Process Management (killProcess, isProcessRunning)
├── Configuration (loadConfig, validation)
└── CLI Interface (main, printUsage)
```

### Testowanie
```bash
# Unit testy
go test -v

# Integration testy
./test_scenarios.sh

# Load test
for i in {1..10}; do ./monitor_mutex "echo test" "/tmp/test$i.log" & done
```
