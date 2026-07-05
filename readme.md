# 🚀 ServeProxy

**ServeProxy** adalah aplikasi CLI berbasis TUI (Text User Interface) yang ditulis dalam bahasa Go (Golang). Aplikasi ini dirancang khusus untuk lingkungan Linux sebagai alternatif lokal pengganti Laravel Valet atau Laravel Herd. 

ServeProxy mengelola proses `php artisan serve` dan server built-in PHP (`php -S`) di latar belakang, mengalokasikan port unik secara dinamis, dan menghubungkannya dengan Nginx (reverse proxy) sehingga proyek web lokal Anda dapat diakses menggunakan domain kustom berakhiran `.test` (seperti `http://nama-proyek.test`).

---

## ✨ Fitur Utama

- **Auto-Onboarding & Persistent Config:** Saat pertama kali dijalankan, aplikasi akan meminta folder projects Anda dan menyimpannya di file konfigurasi JSON dalam direktori standard user config (`~/.config/serveproxy/config.json`).
- **Dukungan Multi-Project PHP:**
  - **Laravel:** Mendeteksi folder yang berisi file `artisan` (server dijalankan dengan `php artisan serve --port={port}`).
  - **PHP Built-in:** Mendeteksi folder berisi file `index.php` atau `public/index.php` (server dijalankan dengan `php -S 127.0.0.1:{port}`).
- **Multi PHP Version per Project:** Setiap project dapat menggunakan versi PHP yang berbeda. ServeProxy mendeteksi versi PHP yang tersedia (`~/.config/herd-lite/bin/php*`) dan mengizinkan Anda mengatur versi per project melalui TUI (tombol `v`) atau file `.php-version`.
- **Dynamic Port Allocation:** Setiap project yang diaktifkan akan dialokasikan ke port unik mulai dari `8000` secara berurutan tanpa risiko bentrok.
- **Tampilan Tabel Rapi (Tidy UI):** Tampilan tabel daftar project tersusun rapi dengan lebar kolom teratur dan informasi status server serta URL lokal.
- **Nginx Integration (Zero Port URL):** Menghilangkan port pada browser. Cukup akses menggunakan domain `.test`.
- **Resource Cleanup Aman:** Saat aplikasi ditutup, semua proses PHP/Laravel di latar belakang (*child processes*) otomatis dihentikan bersih.

---

## 🛠️ Kebutuhan Sistem & Persiapan

### 1. Instalasi PHP & Composer (Cara Cepat)
Jika sistem Linux Anda belum terpasang PHP dan Composer, Anda bisa memasangnya dengan mudah dan cepat menggunakan perintah resmi berikut:
```bash
/bin/bash -c "$(curl -fsSL https://php.new/install/linux)"
```
> [!IMPORTANT]
> Jangan lupa untuk menambahkan `$HOME/.config/herd-lite/bin` ke dalam variabel lingkungan `$PATH` di dalam file konfigurasi shell Anda (`.bashrc` atau `.zshrc`) agar biner `php` dapat dieksekusi secara global:
> ```bash
> export PATH="$HOME/.config/herd-lite/bin:$PATH"
> ```

### 2. DNS Lokal (Dnsmasq) & Nginx
Pastikan semua domain berakhiran `.test` diarahkan ke IP lokal (`127.0.0.1`) dan Nginx terpasang:
```bash
sudo apt update && sudo apt install dnsmasq nginx -y
echo "address=/.test/127.0.0.1" | sudo tee /etc/dnsmasq.d/dev-domains.conf
sudo systemctl restart dnsmasq
```

### 2. Konfigurasi Nginx Dynamic Map
Buat berkas konfigurasi virtual host wildcard baru di `/etc/nginx/sites-available/serveproxy`:
```nginx
map $host $backend_port {
    hostnames;
    include /etc/nginx/tui_ports.map;
    default 8000; 
}

server {
    listen 80;
    server_name *.test;

    location / {
        proxy_pass http://127.0.0.1:$backend_port;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_redirect off;
    }
}
```

Aktifkan konfigurasi virtual host tersebut dan siapkan file peta port:
```bash
sudo ln -s /etc/nginx/sites-available/serveproxy /etc/nginx/sites-enabled/
sudo touch /etc/nginx/tui_ports.map
sudo chmod 666 /etc/nginx/tui_ports.map
sudo systemctl restart nginx
```

### 3. Izin Reload Nginx Tanpa Sudo
Agar ServeProxy dapat memerintahkan Nginx memuat ulang peta port secara otomatis tanpa meminta password root/sudo, jalankan:
```bash
sudo chmod u+s /usr/sbin/nginx
```

### 4. Instalasi Multi-Version PHP (Opsional)
Untuk menggunakan fitur multi PHP version per project, Anda perlu menginstal beberapa versi PHP ke path yang berbeda. php.new menyediakan beberapa versi PHP dari CDN mereka:

```bash
# PHP 8.5 (latest)
curl -L "https://download.herdphp.com/herd-lite/linux/x64/8.5/php" -o ~/.config/herd-lite/bin/php8.5

# PHP 8.4
curl -L "https://download.herdphp.com/herd-lite/linux/x64/8.4/php" -o ~/.config/herd-lite/bin/php8.4

# PHP 8.3 (default dari php.new)
curl -L "https://download.herdphp.com/herd-lite/linux/x64/php" -o ~/.config/herd-lite/bin/php8.3

chmod +x ~/.config/herd-lite/bin/php*
```

> [!NOTE]
> CDN php.new saat ini hanya menyediakan PHP 8.3, 8.4, dan 8.5. Untuk versi lebih lama (8.2, 8.1, 8.0, 7.4), Anda perlu mengunduh dari sumber lain (misalnya: [ondrej/php PPA](https://ppa.launchpadcontent.net/ondrej/php/ubuntu) atau kompilasi manual dari [php.net](https://www.php.net/downloads.php)).

---

## 🚀 Instalasi & Kompilasi

1. Pastikan Anda memiliki compiler Go terinstal di sistem Anda.
2. Unduh dependensi dan kompilasi proyek:
   ```bash
   go mod tidy
   go build -o serveproxy main.go
   ```
3. Pindahkan biner ke folder sistem agar bisa dijalankan secara global:
   ```bash
   sudo mv serveproxy /usr/local/bin/
   ```

---

## 🎮 Cara Penggunaan

Cukup jalankan perintah berikut di terminal Anda:
```bash
serveproxy
```

### Proses Onboarding (Pertama Kali Dijalankan)
Jika file konfigurasi belum ada, Anda akan diminta memasukkan path absolut folder project Anda (misal: `/home/user/Projects` atau `~/Projects`). Jalur ini akan disimpan di `~/.config/serveproxy/config.json`.

### Navigasi & Kontrol TUI
- **`↑ / ↓`** atau **`k / j`** : Navigasi menaikkan/menurunkan pilihan proyek pada tabel.
- **`Enter`** atau **`Spasi`** : Menyalakan (`● RUNNING`) atau mematikan (`○ OFF`) server proyek yang dipilih.
- **`v`** : Mengubah versi PHP untuk proyek yang dipilih (akan muncul picker versi PHP yang tersedia).
- **`Ctrl+C`** : Keluar dari aplikasi (Otomatis mematikan seluruh proses server proyek yang berjalan di latar belakang).
  > **Catatan:** Tombol pintas `q` sengaja dinonaktifkan untuk mencegah konflik penutupan aplikasi ketika Anda mengetik karakter "q" saat proses onboarding/pengaturan folder.

---

## ⚙️ Konfigurasi PHP Version

### Cara Mengatur PHP Version per Project

1. **Melalui TUI (Recommended):**
   - Jalankan `serveproxy`
   - Navigasi ke project yang ingin diubah versi PHP-nya
   - Tekan tombol **`v`** untuk membuka PHP version picker
   - Pilih versi PHP yang diinginkan (atau pilih "(default)" untuk menggunakan versi default)
   - Tekan **Enter** untuk konfirmasi, **Esc** untuk membatalkan

2. **Melalui File `.php-version`:**
   - Buat file bernama `.php-version` di root project
   - Isi dengan versi PHP yang diinginkan (contoh: `8.4`)
   - ServeProxy akan membaca file ini secara otomatis saat memindai project

3. **Melalui Config JSON:**
   - Edit file `~/.config/serveproxy/config.json`
   - Tambahkan field `php_versions` dan `default_php_version`:

   ```json
   {
     "projects_dir": "~/Projects",
     "php_path": "/home/user/.config/herd-lite/bin/php",
     "default_php_version": "8.4",
     "php_versions": {
       "my-laravel-app": "8.4",
       "legacy-app": "8.2",
       "api-service": "8.3"
     }
   }
   ```

### Prioritas Penentuan PHP Version

ServeProxy menentukan versi PHP untuk setiap project dengan urutan prioritas berikut:
1. **Config `php_versions`** (paling tinggi)
2. **File `.php-version`** di root project
3. **Config `default_php_version`**
4. **Default PHP path** (`~/.config/herd-lite/bin/php`)

---

## 💡 Troubleshooting

### 1. Error "502 Bad Gateway" pada Browser
* **Penyebab:** Konfigurasi DNS/Nginx belum reload atau alamat IP binding bermasalah.
* **Solusi:** ServeProxy telah menggunakan IP binding default `127.0.0.1` (bukan `localhost`) untuk PHP built-in server agar kompatibel penuh dengan Nginx. Pastikan status file `/etc/nginx/tui_ports.map` terisi dan dapat ditulis (`chmod 666`).

### 2. Browser Mengarahkan ke Google Search / HTTPS
* Ketik alamat lengkap beserta skema protokolnya (contoh: `http://laravelapp.test`).
* Jika browser tetap memaksa mengarahkan ke HTTPS, gunakan mode Penyamaran (**Incognito Window**) untuk menghindari cache HSTS browser Anda.
