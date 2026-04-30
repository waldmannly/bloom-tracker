# 🌸 Bloom — Period Tracker

**Your cycle, your data, your privacy. No ads. No tracking. No BS.**

Bloom is a free, open-source, self-hosted period tracking app. One binary. No cloud. No subscriptions. Just a simple, beautiful tool that respects your privacy.

Built for people who want to understand their bodies — and partners who want to be supportive.

## ✨ Features

### Core Tracking
- 📅 **Period logging** with start/end dates and cycle history
- 📝 **30+ symptoms** across physical, emotional, flow categories — plus custom symptoms
- 🔢 **Multi-symptom logging** — select several symptoms at once, all saved with one click
- 🌡️ **Basal body temperature** (BBT) daily tracking
- 💧 **Cervical mucus** tracking (symptothermal method support)
- 😴 **Daily wellness** — sleep quality, stress level, energy level (1-5 scales)
- 📔 **Daily journal** with mood emoji picker

### Intelligence
- 🔮 **Cycle predictions** — next period, ovulation, fertile window
- 📊 **Trends & analytics** — cycle length charts, symptom patterns, month-by-month grid
- 🔬 **Phase-symptom correlation** heat map
- 🌡️ **BBT temperature chart** with mucus overlay
- ⚠️ **Smart alerts** — irregular cycles, late periods, severity spikes
- 🧘 **Wellness tips** — phase-specific exercise, nutrition, and nutrient guidance
- 💛 **Encouragement** — affirmations and treat ideas per cycle phase

### Partner Support
- 💑 **Partner dashboard** — invite your partner with a code, they get their own view
- 📧 **Weekly partner emails** — phase updates, action items, treat suggestions
- 🚨 **Instant symptom alerts** — partner is notified immediately when severity is high (4-5)
- 🔄 **Phase change alerts** — partner is emailed when your cycle phase changes

### Privacy & Data
- 🔒 **100% local** — SQLite database, nothing leaves your server
- 🛡️ **Database encryption** — optional AES-256-GCM at-rest encryption with your own key
- 🔐 **Encrypted backups** — AES-256-GCM with password protection
- 📦 **Data export** — download everything as CSV or JSON
- 🗑️ **Account deletion** — full data wipe with password confirmation
- 📖 **Transparent calculations** — see exactly how predictions are made
- 🌿 **Fertility toggle** — hide fertility data if you don't want it
- 🏳️‍🌈 **Inclusive** — pronoun settings (she/her, he/him, they/them)

### Import & Migration
- 📥 **Bulk import** — paste period history in flexible date formats
- 📄 **Import template** — downloadable CSV template
- 📤 **Backup restore** — restore encrypted backups to any Bloom instance

## 🚀 Quick Start

### Option 0: Try It Now (Hosted Demo)

Don't want to install anything? We host a free instance you can use right now:

**👉 [bloom.gorillawiz.com](https://bloom.gorillawiz.com)**

> ⚠️ **Privacy note:** This is a shared hosted instance. While we encrypt the database at rest and don't sell or share data, your information lives on our server — not yours. For maximum privacy, [self-host Bloom](#-self-hosting-guide). It takes about 5 minutes.

### Option 1: Docker (Recommended)

```bash
git clone https://github.com/waldmannly/bloom-tracker.git
cd bloom
cp .env.example .env
docker compose up -d
```

Open **http://localhost:8080** and create your account.

### Option 2: Build from Source

```bash
git clone https://github.com/waldmannly/bloom-tracker.git
cd bloom
go build -o bloom .
./bloom
```

### Option 3: Download Binary

Grab the latest release from the [Releases](https://github.com/waldmannly/bloom-tracker/releases) page. Run it. Done.

## ⚙️ Configuration

All settings are optional. Bloom works with zero configuration.

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server port |
| `DB_PATH` | `period_tracker.db` | SQLite database file path |
| `ENCRYPTION_KEY` | _(disabled)_ | Passphrase for database-at-rest encryption (8+ chars). See [Database Encryption](#-database-encryption) |
| `EMAIL_API_URL` | _(disabled)_ | URL for HTTP email service (e.g. `http://127.0.0.1:3100/send`) |
| `EMAIL_API_KEY` | _(disabled)_ | API key for the email service |
| `SMTP_HOST` | `smtp.gmail.com` | SMTP server (fallback if no API configured) |
| `SMTP_PORT` | `587` | SMTP port |
| `SMTP_EMAIL` | _(disabled)_ | Gmail address for sending notifications |
| `SMTP_PASS` | _(disabled)_ | Gmail App Password ([create one here](https://myaccount.google.com/apppasswords)) |

Copy `.env.example` to `.env` and fill in what you need.

## 🏗️ Architecture

Bloom is intentionally simple:

```
bloom (single Go binary)
├── SQLite database (local file)
├── Embedded HTML templates
├── Embedded CSS + JS
└── No external dependencies at runtime
```

- **Language:** Go (1.21+)
- **Database:** SQLite (via modernc.org/sqlite — pure Go, no CGO)
- **Auth:** bcrypt passwords, secure session cookies
- **Templates:** Go html/template with embedded filesystem
- **Email:** SMTP or HTTP API (optional, for partner notifications)
- **Encryption:** AES-256-GCM with PBKDF2 key derivation (for backups and database-at-rest encryption)

No React. No Node. No Docker required. No cloud services. Just one binary that embeds everything.

## 🔒 Privacy

- **Self-hosted = full privacy.** Data on your hardware, under your control. Nobody else can access it.
- **Hosted instances are convenient** but your data lives on someone else's server. You're trusting the operator.
- No analytics, telemetry, or tracking scripts
- No third-party API calls or cloud sync
- No data selling — ever
- Optional database-at-rest encryption with your own key
- Encrypted backups use AES-256-GCM (your password never leaves your browser)
- Full data export (CSV/JSON) and account deletion at any time
- Bloom is fully open-source — verify what the code does, then self-host for maximum privacy
- See `/privacy` in the app for the full policy

## 🛡️ Database Encryption

Bloom supports encrypting the entire SQLite database file at rest using AES-256-GCM with a key you provide. When enabled:

1. **Startup:** The encrypted `.enc` file is decrypted to a working `.db` file
2. **Running:** Periodic encrypted snapshots every 5 minutes (crash resilience)
3. **Shutdown:** The database is encrypted and the plaintext `.db` file is removed

### Enable It

```bash
# Set a strong passphrase (8+ characters)
export ENCRYPTION_KEY="your-secret-passphrase-here"
./bloom
```

Or in your `.env` file:

```
ENCRYPTION_KEY=your-secret-passphrase-here
```

### How It Works

- Key derivation: PBKDF2 with SHA-256, 100,000 iterations, random 16-byte salt
- Encryption: AES-256-GCM (authenticated encryption)
- The encrypted file uses a `BLOMD` header format (distinct from backup files)
- If the app crashes, the `.enc` file is at most 5 minutes old
- If the unencrypted `.db` file exists on startup (crash recovery), it's used directly

### ⚠️ Important

- **If you lose your encryption key, your data is gone.** There is no recovery.
- The key minimum is 8 characters, but use something much stronger.
- The database is only encrypted at rest — while Bloom is running, the working copy is unencrypted in memory/on disk.

## 🤝 For Partners

Partners get their own login and a dedicated dashboard showing:
- Current cycle phase with supportive tips
- Action items ("bring her a heating pad", "plan a fun date")
- Treat ideas matched to the current phase
- Wellness tips (exercise, nutrition)
- Words of encouragement

Partners **cannot** edit data — they can only view what's shared.

## 📖 How Predictions Work

Bloom uses transparent, documented math — no black-box AI:

- **Ovulation:** Cycle length − 14 (calendar method)
- **Fertile window:** Ovulation day −5 through +1
- **Advanced:** BBT temperature shift + cervical mucus patterns (symptothermal method, 99.6% effective)

Full methodology at `/methodology` in the app.

## 🛠️ Self-Hosting Guide

Bloom is a single binary with zero runtime dependencies. You can run it on anything — a Raspberry Pi, an old laptop, a NAS, or a cloud VPS.

### Prerequisites

- A computer or server that stays on (Linux, macOS, or Windows)
- ~20 MB of disk space
- [Go 1.21+](https://go.dev/dl/) (only if building from source)
- (Optional) A domain name, if you want to access it from outside your network
- (Optional) Docker, if you prefer containers

---

### Step 1: Install Bloom

Pick whichever method you prefer:

#### Option A — Download the Binary (Easiest)

```bash
# Download the latest release (Linux example)
curl -L https://github.com/waldmannly/bloom-tracker/releases/latest/download/bloom-linux-amd64 -o bloom
chmod +x bloom
```

#### Option B — Docker

```bash
git clone https://github.com/waldmannly/bloom-tracker.git
cd bloom
cp .env.example .env        # edit .env if you want email or encryption
docker compose up -d
```

Skip to [Step 4](#step-4-open-bloom) — Docker handles the rest.

#### Option C — Build from Source

Requires [Go 1.21+](https://go.dev/dl/).

```bash
git clone https://github.com/waldmannly/bloom-tracker.git
cd bloom
go build -o bloom .
```

---

### Step 2: Configure (Optional)

Bloom works with zero configuration. But if you want email, encryption, or a custom port:

```bash
cp .env.example .env
nano .env    # or open in any text editor
```

Key settings:

```bash
PORT=8080                          # change if 8080 is taken
DB_PATH=./data/bloom.db            # where your data lives
ENCRYPTION_KEY=my-secret-key       # encrypt the database at rest (optional)

# Email — pick ONE method:
# Gmail SMTP:
SMTP_EMAIL=you@gmail.com
SMTP_PASS=abcd-efgh-ijkl-mnop     # Gmail App Password, NOT your real password
# Or an HTTP email service:
EMAIL_API_URL=http://127.0.0.1:3100/send
EMAIL_API_KEY=your-api-key
```

> **Gmail App Passwords:** Go to https://myaccount.google.com/apppasswords, generate one for "Mail", and paste it as `SMTP_PASS`. This only works if you have 2FA enabled on your Google account.

---

### Step 3: Run It

```bash
./bloom
```

You should see:

```
🌸 Bloom Period Tracker running on http://localhost:8080
```

---

### Step 4: Open Bloom

Open a browser and go to **http://localhost:8080**. Create your account. That's it — you're tracking.

---

## 🌐 Accessing Bloom From Anywhere

By default, Bloom is only reachable from the machine it runs on. Here's how to open it up — from simplest to most robust.

### Option 1: Access From Other Devices on Your Home Network (LAN)

No configuration needed. Just find the IP address of the machine running Bloom:

```bash
# Linux/macOS
hostname -I

# Windows
ipconfig
```

Then from any device on the same Wi-Fi/network, open:

```
http://192.168.1.XXX:8080
```

Replace `192.168.1.XXX` with your machine's local IP.

> **Tip:** This works for phones, tablets, and your partner's laptop — as long as everyone is on the same network.

---

### Option 2: Tailscale / WireGuard VPN (Recommended for Personal Use)

This is the easiest and most secure way to reach Bloom from anywhere — without exposing it to the public internet.

1. Install [Tailscale](https://tailscale.com/) (free for personal use) on the machine running Bloom and on your phone/laptop.
2. Both devices join the same Tailscale network automatically.
3. Access Bloom via your Tailscale IP:

```
http://100.x.y.z:8080
```

**Why this is great:**
- 🔒 Encrypted tunnel — nobody else can see it
- 🚫 No ports to open on your router
- 📱 Works from anywhere (coffee shop, work, travel)
- ⚡ Takes about 5 minutes to set up

---

### Option 3: Reverse Proxy + Domain (For Public or Shared Access)

If you want a real URL like `https://bloom.yourdomain.com` — for example, to share with a partner who lives elsewhere — you'll need:

1. A **domain name** (or subdomain) pointing to your server's public IP
2. A **reverse proxy** (Nginx or Caddy) to handle HTTPS
3. **Port forwarding** on your router (if hosting at home) — or use a cloud VPS

#### Using Nginx + Let's Encrypt (Free SSL)

```bash
# Install nginx and certbot
sudo apt install nginx certbot python3-certbot-nginx
```

Create `/etc/nginx/sites-available/bloom`:

```nginx
server {
    listen 80;
    server_name bloom.yourdomain.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Enable it and get a free SSL certificate:

```bash
sudo ln -s /etc/nginx/sites-available/bloom /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
sudo certbot --nginx -d bloom.yourdomain.com
```

Certbot will automatically configure HTTPS and set up auto-renewal. You're done.

#### Using Caddy (Even Simpler)

[Caddy](https://caddyserver.com/) handles SSL automatically with zero configuration:

```bash
# Install Caddy
sudo apt install caddy
```

Edit `/etc/caddy/Caddyfile`:

```
bloom.yourdomain.com {
    reverse_proxy 127.0.0.1:8080
}
```

```bash
sudo systemctl reload caddy
```

That's literally it. Caddy auto-provisions SSL certificates.

#### Home Router Port Forwarding

If you're hosting at home (not a VPS), you need to forward ports through your router:

1. Log into your router (usually `192.168.1.1` or `192.168.0.1`)
2. Find **Port Forwarding** (sometimes under NAT, Firewall, or Advanced)
3. Forward port **80** and **443** (TCP) to your server's local IP
4. Point your domain's DNS A record to your home's public IP

> **Dynamic IP?** Most home ISPs change your IP periodically. Use a free dynamic DNS service like [DuckDNS](https://www.duckdns.org/) or [Cloudflare Tunnels](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/) to keep your domain pointing to the right place.

---

### Option 4: Host on a Cloud VPS (Easiest for Public Access)

If you don't want to deal with your home network, rent a small VPS:

| Provider | Cheapest Plan | Notes |
|----------|--------------|-------|
| [Hetzner](https://hetzner.com/cloud) | ~€3.50/mo | Best value, EU-based (GDPR ✓) |
| [DigitalOcean](https://digitalocean.com) | $4/mo | Simple, great docs |
| [Linode](https://linode.com) | $5/mo | Reliable |
| [Oracle Cloud](https://cloud.oracle.com) | **Free tier** | ARM instance, always free |
| [Fly.io](https://fly.io) | **Free tier** | Good for small apps |

**Setup on a fresh VPS:**

```bash
# Upload Bloom
scp bloom you@your-server:/opt/bloom/bloom

# SSH in
ssh you@your-server

# Create data directory
sudo mkdir -p /opt/bloom/data
sudo chown $USER:$USER /opt/bloom /opt/bloom/data

# Create .env
cat > /opt/bloom/.env << 'EOF'
PORT=8080
DB_PATH=/opt/bloom/data/bloom.db
EOF

# Run it (see "Run as a Service" below to keep it running)
cd /opt/bloom && ./bloom
```

Then set up Nginx or Caddy (see Option 3 above) for HTTPS.

---

## 🔧 Run as a Service (Keep Bloom Running)

Don't run Bloom in a terminal that closes when you log out. Set it up as a system service so it starts on boot and restarts if it crashes.

### Linux (systemd)

Create `/etc/systemd/system/bloom.service`:

```ini
[Unit]
Description=Bloom Period Tracker
After=network.target

[Service]
Type=simple
ExecStart=/opt/bloom/bloom
WorkingDirectory=/opt/bloom
EnvironmentFile=/opt/bloom/.env
Restart=always
RestartSec=5

# Security hardening (optional but recommended)
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/opt/bloom/data

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable bloom     # start on boot
sudo systemctl start bloom      # start now
sudo systemctl status bloom     # check it's running
```

### macOS (launchd)

Create `~/Library/LaunchAgents/com.bloom.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>com.bloom.tracker</string>
    <key>ProgramArguments</key>
    <array><string>/opt/bloom/bloom</string></array>
    <key>WorkingDirectory</key><string>/opt/bloom</string>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
</dict>
</plist>
```

```bash
launchctl load ~/Library/LaunchAgents/com.bloom.plist
```

### Windows

Use [NSSM](https://nssm.cc/) to run Bloom as a Windows service:

```powershell
nssm install Bloom C:\bloom\bloom.exe
nssm set Bloom AppDirectory C:\bloom
nssm start Bloom
```

Or just run `bloom.exe` at startup (simpler but less robust).

---

## 💾 Backups

### Manual (From the App)

Go to **Settings → Encrypted Backup**. This downloads a password-protected `.enc` file you can store wherever you want. Restore it on any Bloom instance via **Settings → Restore Backup**.

### Automated (Cron)

```bash
# Daily backup at 3 AM, keep 30 days
0 3 * * * cp /opt/bloom/data/bloom.db /backups/bloom-$(date +\%Y\%m\%d).db
0 4 * * * find /backups -name "bloom-*.db" -mtime +30 -delete
```

### Off-Site (Encrypted)

If database encryption is enabled (`ENCRYPTION_KEY`), the `.enc` file is safe to sync anywhere:

```bash
# Sync encrypted database to cloud storage
0 3 * * * cp /opt/bloom/data/bloom.db.enc ~/Dropbox/backups/bloom.enc
```

---

## 🔐 Security Checklist

If you're exposing Bloom to the internet, go through this list:

- [ ] **HTTPS only** — use Nginx/Caddy with SSL (Let's Encrypt is free)
- [ ] **Strong passwords** — Bloom enforces minimum 8 characters, but use more
- [ ] **Database encryption** — set `ENCRYPTION_KEY` so the database file is encrypted at rest
- [ ] **Firewall** — only open ports 80 and 443 (block 8080 from the public)
- [ ] **Updates** — keep your server OS and Bloom binary up to date
- [ ] **Backups** — automate daily backups (see above)

```bash
# UFW firewall example (Ubuntu)
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow ssh
sudo ufw enable
```

## 🌱 Contributing

Bloom is built with love. Contributions welcome!

1. Fork the repo
2. Create a feature branch (`git checkout -b feature/amazing`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing`)
5. Open a Pull Request

## ❓ FAQ

**Q: Do I need Docker?**
No. Bloom is a single binary — just download and run it. Docker is an option, not a requirement.

**Q: Does it work on Raspberry Pi?**
Yes! Cross-compile with `GOOS=linux GOARCH=arm64 go build -o bloom .` or `GOARCH=arm` for older Pi models.

**Q: Can multiple users share one instance?**
Yes. Each user creates their own account. Data is isolated per user. Partners link via invite codes.

**Q: What happens if I forget my encryption key?**
Your data is gone. There is no recovery, no backdoor, no reset. This is by design — we can't access your data even if we wanted to.

**Q: Is this a medical device?**
No. Bloom is an awareness tool. It uses documented math for predictions but is not FDA-approved or medically validated. Always consult a healthcare provider for medical decisions.

**Q: Can I migrate from Clue/Flo/another app?**
Export your data from the other app (most support CSV), then use Bloom's Import page to paste your period dates. The format is flexible — `2024-01-15 to 2024-01-20` works, so does `Jan 15 2024 - Jan 20 2024`.

**Q: How do partner notifications work?**
The cycle owner enables notifications in Settings. The partner gets a weekly summary email every Monday, plus instant alerts when severe symptoms (4-5) are logged or the cycle phase changes.

## 📝 License

[MIT](LICENSE) — do whatever you want with it. Free forever.

---

**Made with 💛 for anyone who wants to understand their body without giving up their privacy.**
