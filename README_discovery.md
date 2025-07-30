# Monitor Discovery - Dokumentacja

## Opis

Monitor Discovery to narzędzie do automatycznego wykrywania procesów w systemie Linux, które mogą być monitorowane przez monitor_mutex. Program skanuje system i sugeruje konfigurację dla znalezionych procesów.

## Funkcje

### 🔍 Automatyczne wykrywanie procesów
- **Procesy z plikami logów** - używa `lsof` do znajdowania procesów piszących do plików .log
- **Procesy nasłuchujące** - używa `ss`/`netstat` do znajdowania usług sieciowych
- **Długo działające procesy** - znajduje procesy działające dłużej niż godzinę

### 🛡️ Filtrowanie bezpieczeństwa
- Automatycznie pomija procesy systemowe (systemd, kernel, dbus, itp.)
- Sprawdza czy proces można bezpiecznie restartować
- Preferuje procesy użytkowników przed procesami root

### ⚙️ Inteligentna konfiguracja
- Automatycznie dostosowuje timeout i interval do typu procesu
- Generuje bezpieczne ścieżki do plików logów
- Tworzy gotową konfigurację YAML

## Instalacja

```bash
# Kompilacja
go build -o discovery discovery.go

# Lub bezpośrednie uruchomienie
go run discovery.go
```

## Użycie

### Podstawowe uruchomienie
```bash
./discovery
```

Program przeprowadzi przez interaktywny proces:
1. Zeskanuje system w poszukiwaniu procesów
2. Wyświetli listę kandydatów
3. Pozwoli wybrać procesy do monitorowania
4. Wygeneruje konfigurację

### Przykład sesji
```
🔍 Skanowanie systemu w poszukiwaniu procesów...
   Znaleziono 5 procesów z plikami logów
   Znaleziono 3 procesów nasłuchujących
   Znaleziono 8 długo działających procesów
   Łącznie: 12 unikalnych kandydatów

📋 Znalezione kandydaci do monitorowania:
==========================================
[ 1] nginx                PID: 1234     Port: 80     
     Log: /var/log/nginx/access.log
     Cmd: nginx: master process /usr/sbin/nginx

[ 2] python3              PID: 5678     CPU: 2.1%  
     Log: /tmp/app.log
     Cmd: python3 /home/user/myapp.py

Wybierz numery procesów (np: 1,2 lub 'all'): 1,2
```

### Wybór procesów
- **Konkretne numery**: `1,3,5` - wybiera procesy 1, 3 i 5
- **Wszystkie**: `all` - wybiera wszystkie znalezione procesy
- **Puste**: Enter - kończy bez wyboru

## Wygenerowana konfiguracja

Program tworzy plik YAML gotowy do użycia z monitor_mutex:

```yaml
# Automatycznie wygenerowana konfiguracja monitora
# Data: 2024-01-15 14:30:25

processes:
  - name: "nginx"
    command: "nginx: master process /usr/sbin/nginx"
    log_file: "/var/log/nginx/access.log"
    timeout: 120
    interval: 10

  - name: "python3"
    command: "python3 /home/user/myapp.py"
    log_file: "/tmp/app.log"
    timeout: 45
    interval: 8
```

## Automatyczne ustawienia

### Timeout i interval według typu procesu:

| Typ procesu | Timeout | Interval | Przykłady |
|-------------|---------|----------|-----------|
| Skrypty | 45s | 8s | python, bash, node, ruby |
| Serwery web | 120s | 10s | nginx, apache, httpd |
| Bazy danych | 180s | 15s | mysql, postgres, redis |
| Aplikacje Java | 300s | 20s | *.jar, java |
| Serwery sieciowe | 90s | 10s | procesy z portami |
| Domyślne | 60s | 5s | inne procesy |

### Bezpieczeństwo

Automatycznie **pomijane** procesy:
- Procesy systemowe (systemd, kernel, dbus)
- Procesy w katalogach systemowych (/usr/lib/systemd/, /sbin/)
- Procesy krytyczne (init, ssh, getty, mount)

**Preferowane** procesy:
- Procesy użytkowników (nie root)
- Procesy w /home/, /opt/, /usr/local/, /tmp/
- Aplikacje użytkownika

## Wymagania systemowe

### Obowiązkowe
- Linux (testowane na Ubuntu/Debian)
- Go 1.16+ (do kompilacji)

### Opcjonalne (dla pełnej funkcjonalności)
- `lsof` - do wykrywania procesów z logami
- `ss` lub `netstat` - do wykrywania procesów sieciowych
- `ps` - do analizy długo działających procesów

### Instalacja narzędzi (Ubuntu/Debian)
```bash
sudo apt update
sudo apt install lsof net-tools procps
```

## Rozwiązywanie problemów

### Brak znalezionych procesów
```
❌ Nie znaleziono kandydatów do monitorowania
```

**Rozwiązania:**
1. Uruchom więcej aplikacji użytkownika
2. Utwórz testowy proces:
   ```bash
   nohup bash -c 'while true; do echo $(date) Test app; sleep 10; done > /tmp/test.log' &
   ```

### Błąd "lsof niedostępne"
```
⚠️ lsof niedostępne, używam metody fallback...
```

**Rozwiązanie:**
```bash
sudo apt install lsof
```

### Wszystkie procesy zostały odrzucone
```
🚫 Pominięto proces systemowy: systemd
⚠️ Pominięto niebezpieczny proces: ssh
```

To normalne - program chroni przed monitorowaniem procesów systemowych.

## Przykłady użycia

### Testowanie z przykładowymi procesami
```bash
# Utwórz testowe procesy
nohup python3 -c "
import time
while True:
    print(f'{time.ctime()}: App running')
    time.sleep(5)
" > /tmp/test_app.log 2>&1 &

nohup bash -c "
while true; do
    echo \$(date): Service active
    sleep 10
done > /tmp/test_service.log
" &

# Uruchom discovery
./discovery
```

### Monitoring aplikacji webowej
```bash
# Uruchom prostą aplikację Flask
nohup python3 -m flask run --host=0.0.0.0 --port=5000 > /tmp/flask.log 2>&1 &

# Discovery znajdzie ją jako proces z portem 5000
./discovery
```

## Integracja z monitor_mutex

Po wygenerowaniu konfiguracji:

```bash
# Użyj wygenerowanej konfiguracji
./monitor_mutex --config monitor_config.yaml

# Lub z dodatkowymi opcjami
./monitor_mutex --config monitor_config.yaml --verbose
```

## Pliki wyjściowe

- **monitor_config.yaml** - domyślna nazwa konfiguracji
- **custom_name.yaml** - można podać własną nazwę
- Program automatycznie dodaje rozszerzenie .yaml jeśli brakuje

## Wskazówki

### Najlepsze praktyki
1. **Uruchom discovery jako zwykły użytkownik** - znajdzie bezpieczniejsze procesy
2. **Sprawdź wygenerowaną konfigurację** przed użyciem z monitorem
3. **Testuj na procesach nieprodukcyjnych** najpierw

### Optymalizacja wykrywania
1. Uruchom własne aplikacje przed scanowaniem
2. Użyj katalogów /tmp/ lub /home/ dla testów
3. Upewnij się że aplikacje piszą logi

---

**Autor:** Monitor Discovery Tool  
**Wersja:** 1.0  
**Licencja:** Do użytku wewnętrznego
