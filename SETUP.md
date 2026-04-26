# AVANGARD — пошаговая установка

Этот документ — **что делать руками, чтобы поднять рабочий AVANGARD**: VPS, сервер, клиент, проверка обхода. Если ты впервые — иди по разделам подряд.

> Краткая версия:
> ```
> # на VPS
> curl -fsSL https://raw.githubusercontent.com/e1mxro/avangard-private/init/deploy/install.sh \
>   | sudo DOMAIN=tunnel.example.com EMAIL=you@example.com bash
> # на клиенте
> avangard-client socks5 --listen 127.0.0.1:1080 'avangard://...URI...из вывода install.sh'
> curl --socks5 127.0.0.1:1080 https://example.com
> ```

---

## 0. Что тебе понадобится

| Компонент | Что | Зачем |
|-----------|-----|-------|
| VPS вне РФ | 1 vCPU / 1 GB RAM минимум, Ubuntu 22.04 / Debian 12 | Конечная точка туннеля |
| Домен | любой, A-запись на VPS | Для Let's Encrypt + camouflage SNI |
| 80 + 443 порты | Открыты на VPS | 80 для ACME challenge, 443 для туннеля |
| Локальная машина | Linux/macOS/Windows + Go ≥ 1.25 *или* собранный бинарь | Клиент |

**Где брать VPS** (в порядке предпочтения для RU-аудитории):
- Hetzner (DE/FI) — дёшево, но иногда DPI-friction.
- Selectel / TimeWeb — RU-relay через partner-network (см. spec §6.4).
- Vultr / DigitalOcean — стабильно, но IP-диапазоны иногда фильтруются.
- Reg.ru cloud — RU-relay, законно как «личное использование».

---

## 1. Подготовка VPS

```bash
ssh root@your-vps-ip
```

### 1.1. DNS

Создай A-запись:

```
tunnel.example.com.   A   <IP вашего VPS>
```

Дождись пропагирования: `dig +short tunnel.example.com` должен вернуть IP.

### 1.2. Файрвол

```bash
ufw allow 22/tcp
ufw allow 80/tcp           # для ACME
ufw allow 443/tcp          # AVANGARD TCP fallback
ufw allow 443/udp          # AVANGARD QUIC основной
ufw enable
```

Если `ufw` нет:
```bash
iptables -I INPUT -p tcp --dport 22 -j ACCEPT
iptables -I INPUT -p tcp --dport 80 -j ACCEPT
iptables -I INPUT -p tcp --dport 443 -j ACCEPT
iptables -I INPUT -p udp --dport 443 -j ACCEPT
```

### 1.3. UDP-буфер для QUIC

```bash
echo 'net.core.rmem_max=7500000' | sudo tee -a /etc/sysctl.d/99-avangard.conf
echo 'net.core.wmem_max=7500000' | sudo tee -a /etc/sysctl.d/99-avangard.conf
sudo sysctl --system
```

Без этого quic-go ругается на маленький UDP receive buffer (ловится в логах).

---

## 2. Установка сервера (одной командой)

```bash
curl -fsSL https://raw.githubusercontent.com/e1mxro/avangard-private/init/deploy/install.sh \
    | sudo DOMAIN=tunnel.example.com EMAIL=you@example.com bash
```

Что произойдёт:

1. `apt-get install` базовых пакетов (`curl`, `git`, `certbot`, `build-essential`).
2. Установится Go 1.25 в `/usr/local/go`.
3. Склонируется репо `e1mxro/avangard-private` в `/tmp`.
4. Соберутся бинари `avangard-server`, `avangard-client` в `/usr/local/bin`.
5. Создастся системный пользователь `avangard` без shell.
6. Получится сертификат Let's Encrypt в `/etc/avangard/{cert,key}.pem`.
7. Сгенерируется приватный ключ Noise + UUID + конфиг `/etc/avangard/server.yaml`.
8. Установится unit `/etc/systemd/system/avangard-server.service`.
9. Запустится `systemctl enable --now avangard-server`.
10. Выведется client URI — скопируй её, понадобится дальше.

### 2.1. Что должно получиться

```bash
systemctl status avangard-server
# Active: active (running)

journalctl -u avangard-server -n 20
# quic listening addr=[::]:443
# tcp listening  addr=[::]:443
```

URI вида:
```
avangard://7d36...c7d9@tunnel.example.com:443?sni=www.yandex.ru&pk=AjOf...&mode=auto&region=auto#avangard
```

### 2.2. Если что-то пошло не так

| Симптом | Причина | Лечение |
|---------|---------|---------|
| `bind: permission denied :443` | Бинарь не имеет CAP_NET_BIND_SERVICE | systemd unit уже даёт capability; проверь `setcap cap_net_bind_service=+ep /usr/local/bin/avangard-server` |
| `certbot: connection refused` | Порт 80 заблокирован файрволом | Открой `ufw allow 80/tcp` |
| `failed to load TLS: open ...cert.pem: no such file` | Не получился ACME, fallback тоже не сработал | `avangard-server selfcert --out /etc/avangard --host tunnel.example.com && systemctl restart avangard-server` |
| QUIC работает, TCP нет | Пакет `tls.Listen` не привязал TCP | проверь `ss -ltnp | grep 443` — должно быть и `LISTEN ... :443` (TCP) и `UNCONN ... :443` (UDP) |

---

## 3. Установка вручную (если install.sh не подходит)

```bash
# Зависимости
apt-get update
apt-get install -y curl git build-essential certbot

# Go
cd /tmp && curl -fsSL https://go.dev/dl/go1.25.1.linux-amd64.tar.gz -o go.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Сборка
git clone https://github.com/e1mxro/avangard-private.git
cd avangard-private
go build -o /usr/local/bin/avangard-server ./cmd/avangard-server
go build -o /usr/local/bin/avangard-client ./cmd/avangard-client

# Сертификат + конфиг
sudo mkdir -p /etc/avangard /var/lib/avangard /var/log/avangard
sudo certbot certonly --standalone -d tunnel.example.com -m you@example.com --agree-tos -n
sudo cp /etc/letsencrypt/live/tunnel.example.com/fullchain.pem /etc/avangard/cert.pem
sudo cp /etc/letsencrypt/live/tunnel.example.com/privkey.pem  /etc/avangard/key.pem
sudo /usr/local/bin/avangard-server keygen \
     --host tunnel.example.com --port 443 --decoy www.yandex.ru \
     --out /etc/avangard

# systemd
sudo useradd --system --home-dir /var/lib/avangard --shell /usr/sbin/nologin avangard
sudo chown -R avangard:avangard /etc/avangard /var/lib/avangard
sudo cp deploy/systemd/avangard-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now avangard-server
sudo journalctl -u avangard-server -f
```

---

## 4. Запуск через Docker (альтернатива systemd)

```bash
git clone https://github.com/e1mxro/avangard-private.git && cd avangard-private
docker build -f deploy/Dockerfile --target server -t avangard-server .

# Сгенерируй ключи + сертификат локально
docker run --rm -v "$PWD/dev:/etc/avangard" --user 0 \
    avangard-server selfcert --out /etc/avangard --host localhost
docker run --rm -v "$PWD/dev:/etc/avangard" --user 0 \
    avangard-server keygen --host 127.0.0.1 --port 443 --decoy localhost --out /etc/avangard

# Запусти сервер
docker run -d --name avangard \
    -p 443:443/tcp -p 443:443/udp \
    -v "$PWD/dev:/etc/avangard:ro" \
    avangard-server
docker logs -f avangard
```

---

## 5. Клиент

### 5.1. Сборка

```bash
git clone https://github.com/e1mxro/avangard-private.git
cd avangard-private
go build -o avangard-client ./cmd/avangard-client
```

(Или скопируй уже собранный бинарь с сервера: `scp root@vps:/usr/local/bin/avangard-client .`)

### 5.2. Подключение

#### Вариант A: локальный SOCKS5 proxy (рекомендуется)

```bash
./avangard-client socks5 \
    --listen 127.0.0.1:1080 \
    --transport tcp \
    'avangard://...URI...'
```

В фоне работает SOCKS5-сервер на `127.0.0.1:1080`. Направляй через него любой софт:

```bash
curl --socks5 127.0.0.1:1080 https://2ip.ru
# должен вернуть IP вашего VPS

# Firefox: about:preferences#general → Network Settings → Manual:
#   SOCKS Host = 127.0.0.1, Port = 1080, SOCKS v5
#   ✓ "Proxy DNS when using SOCKS v5"

# Chrome (Linux):
google-chrome --proxy-server="socks5://127.0.0.1:1080"
```

#### Вариант B: разовый туннель (для отладки)

```bash
echo "GET / HTTP/1.1
Host: example.com

" | ./avangard-client tunnel --transport tcp 'avangard://...URI...' example.com:80
```

#### Вариант C: probe + region report (без подключения)

```bash
./avangard-client probe tunnel.example.com
./avangard-client region --region RU --asn 8359 tunnel.example.com
```

### 5.3. Выбор транспорта

| `--transport` | Когда использовать |
|---|---|
| `tcp` (default) | везде, проверенный, проходит большинство DPI |
| `quic` | если сеть пропускает UDP/443 — быстрее на 30–50 мс RTT |

В будущих версиях клиент сам подбирает транспорт через AutoProbe. Сейчас — флагом.

---

## 6. Проверка что всё работает

С клиентской машины:

```bash
# 1. Probe (без авторизации)
./avangard-client probe tunnel.example.com

# 2. Должно показать:
#    UDP/443    OK=true rtt=...
#    TCP/443    OK=true rtt=...

# 3. Реальная проверка туннеля
./avangard-client socks5 --listen 127.0.0.1:1080 'avangard://...' &

# 4. До туннеля — твой реальный IP:
curl https://api.ipify.org
# 5. Через туннель — IP вашего VPS:
curl --socks5 127.0.0.1:1080 https://api.ipify.org
```

Если IP **разные** — туннель работает.

---

## 7. Эксплуатация

### 7.1. Логи

```bash
journalctl -u avangard-server -f                    # вживую
journalctl -u avangard-server --since "1 hour ago"  # за час
journalctl -u avangard-server -p err                # только ошибки
```

### 7.2. Перезапуск / остановка

```bash
sudo systemctl restart avangard-server
sudo systemctl stop avangard-server
sudo systemctl status avangard-server
```

### 7.3. Обновление

```bash
cd /tmp && rm -rf avangard-private
git clone https://github.com/e1mxro/avangard-private.git
cd avangard-private
go build -o /tmp/avangard-server ./cmd/avangard-server
sudo systemctl stop avangard-server
sudo install -m 0755 /tmp/avangard-server /usr/local/bin/avangard-server
sudo systemctl start avangard-server
```

### 7.4. Ротация Let's Encrypt сертификата

`certbot.timer` автоматически продлевает каждые 90 дней. Чтобы AVANGARD подхватил новый cert:

```bash
sudo crontab -e
# добавь:
30 3 * * * cp /etc/letsencrypt/live/tunnel.example.com/fullchain.pem /etc/avangard/cert.pem && cp /etc/letsencrypt/live/tunnel.example.com/privkey.pem /etc/avangard/key.pem && systemctl reload avangard-server
```

### 7.5. Добавление нового пользователя (UUID)

Открой `/etc/avangard/server.yaml`, добавь новый UUID в `auth.accepted_uuids`, перезапусти. Сгенерировать UUID:

```bash
uuidgen
# или
python3 -c "import uuid; print(uuid.uuid4())"
```

URI для нового пользователя — скопируй существующий и замени UUID:

```
avangard://<NEW-UUID>@tunnel.example.com:443?sni=www.yandex.ru&pk=<тот же pk>&...
```

`pk` (server static pubkey) у всех пользователей одинаковый.

---

## 8. Безопасность

- **Никогда не коммить `/etc/avangard/server.yaml`** в git — там приватный ключ.
- **Регулярно обновляй сервер** (`apt update && apt upgrade`) — особенно ядро Linux и OpenSSL.
- **Не используй один и тот же UUID** на десятках устройств — лучше генерировать по UUID на устройство, чтобы можно было отзывать точечно.
- **MitM-defense:** клиент проверяет `pk` (X25519 server static) при handshake; если кто-то поднимет фейковый сервер, Noise NK обломается.
- **Rotate keys yearly:** `avangard-server keygen` создаст новый `pk`. Раздай новый URI пользователям.

---

## 9. Производительность / тюнинг

- `LimitNOFILE=1048576` уже выставлен в systemd unit.
- Если CPU `100% sys` — проверь, что AES-NI работает (`grep aes /proc/cpuinfo` должен показать `aes`).
- Для high-bandwidth (>500 Mbps) выстави в server.yaml:
  ```yaml
  # будут добавлены в v0.1
  performance:
    quic_send_window: 16777216    # 16 MiB
    quic_recv_window: 16777216
  ```

---

## 10. Что делать если AVANGARD заблокирован

Логика fallback (RegionShield):

1. **QUIC заблокирован** (UDP/443 фильтруется) → клиент сам переходит на TCP/TLS. Ничего делать не надо.
2. **TCP/443 фильтруется по SNI** → используй другой порт: `--transport tcp` уже есть, но запусти сервер на порту 8443/2053/2087/2096 — флагом `PORT=8443` в `install.sh`.
3. **Полный DPI-блок 443** → переключись на WebSocket (`mode=ws` в URI, в разработке) или DNS-туннель (last resort).
4. **Реестр РКН вырубил весь домен** → подними второй VPS на другом провайдере, обнови URI.

См. также `docs/AVANGARD-SPEC.md` §6 (RegionShield) для детальной матрицы режимов.

---

## 11. Удаление

```bash
sudo systemctl stop avangard-server
sudo systemctl disable avangard-server
sudo rm /etc/systemd/system/avangard-server.service
sudo rm /usr/local/bin/avangard-{server,client}
sudo rm -rf /etc/avangard /var/lib/avangard /var/log/avangard
sudo userdel avangard
sudo certbot delete --cert-name tunnel.example.com
```

---

## 12. Помощь

Открой issue на https://github.com/e1mxro/avangard-private/issues с:
- `journalctl -u avangard-server -n 100` (для серверной проблемы)
- `avangard-client probe <host>` JSON-вывод
- Что делал / что ожидал / что получилось
