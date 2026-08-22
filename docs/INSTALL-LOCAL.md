# Panduan Instalasi & Menjalankan Source Code ChatLoop

Panduan langkah-demi-langkah menjalankan project **ChatLoop WhatsApp AI Assistant** di komputer lokal (**macOS**, **Windows**, dan **Linux**).

Versi dokumen PDF: [`docs/PANDUAN-INSTALASI.pdf`](PANDUAN-INSTALASI.pdf).

---

## 1. Instal Software yang Dibutuhkan

Pastikan komputer Anda telah terpasang software pendukung berikut sebelum memulai:

| Software | Fungsi | Tautan Unduhan |
| :--- | :--- | :--- |
| **Visual Studio Code** | Editor kode utama | [code.visualstudio.com](https://code.visualstudio.com/) |
| **Go (Golang)** | Runtime Backend API | [go.dev/dl](https://go.dev/dl/) |
| **Node.js (LTS)** | Runtime Frontend Vite | [nodejs.org](https://nodejs.org/) *(Pilih versi LTS)* |
| **MySQL Database** | Database server | Gunakan **XAMPP**, **Laragon**, atau MySQL Server 8.x |

Setelah instalasi selesai, buka terminal/CMD dan verifikasi:
```bash
go version
node -v
npm -v
```

---

## 2. Ekstrak ZIP & Buka di Visual Studio Code

1. Ekstrak file `ChatLoop-v4-xxxx.zip` yang telah Anda unduh dari Member Dashboard LMS.
2. Buka aplikasi **Visual Studio Code**.
3. Pilih menu **File → Open Folder...** (Windows/Linux) atau **File → Open...** (macOS), lalu pilih folder hasil ekstrak tersebut.
4. Buka Terminal internal VS Code: menu **Terminal → New Terminal** (atau pintasan `Ctrl + \`` / `Cmd + \``).

---

## 3. Membuat Database & Menyiapkan File `.env`

1. Buat database baru di MySQL (via phpMyAdmin / DBeaver / HeidiSQL) dengan nama:
   ```sql
   CREATE DATABASE db_wa_blast;
   ```

2. Di terminal VS Code, jalankan perintah inisialisasi file `.env`:
   ```bash
   npm run setup:env
   ```

   > ✨ **Konfigurasi Otomatis dari LMS:**  
   > File `.env` Anda **sudah otomatis terisi** dengan `LICENSE_KEY` resmi dan `JWT_SECRET` unik Anda. Anda **tidak perlu** generate JWT manual dari website lain atau copy-paste lisensi.

3. Buka file `.env` di sidebar VS Code dan sesuaikan password MySQL lokal Anda:
   ```ini
   DB_HOST=127.0.0.1
   DB_PORT=3306
   DB_USER=root
   DB_PASS=              # Kosongkan jika MySQL lokal Anda tanpa password (default XAMPP)
   DB_NAME=db_wa_blast

   # Nilai di bawah ini sudah otomatis terisi dan siap pakai:
   JWT_SECRET=...
   SUPERADMIN_USERNAME=superadmin
   SUPERADMIN_PASSWORD=superadmin123
   LICENSE_KEY=...
   ```

---

## 4. Instalasi Dependency Project (Hanya 1 Perintah)

Pastikan posisi terminal berada di folder utama project, lalu jalankan:
```bash
npm run setup
```
Perintah ini akan otomatis mengunduh semua modul Go backend dan dependensi React frontend sampai selesai.

---

## 5. Menjalankan Aplikasi

Jalankan backend dan frontend sekaligus dengan perintah:
```bash
npm run dev
```

Aplikasi akan aktif pada port lokal komputer Anda:
* 🌐 **Frontend (Web Dashboard):** `http://127.0.0.1:5173` atau `http://localhost:5173`
* ⚙️ **Backend API:** `http://127.0.0.1:3030`

> ⚠️ *Biarkan terminal tetap terbuka selama Anda menggunakan aplikasi ChatLoop.*

---

## 6. Login ke Dashboard ChatLoop

1. Buka browser dan kunjungi: **`http://localhost:5173`**
2. Login dengan akun bawaan:
   * **Username:** `superadmin`
   * **Password:** `superadmin123`
3. Masuk ke menu **WhatsApp Session** ➔ Scan QR Code dengan WhatsApp di smartphone Anda.

---

## ❓ Penyelesaian Masalah Populer (FAQ)

1. **Perintah `npm` atau `go` tidak dikenali (*command not found*):**
   * Pastikan software sudah terpasang, lalu **tutup dan buka kembali VS Code** agar sistem membaca PATH environment baru.
2. **Gagal koneksi database (*Error: connection refused*):**
   * Pastikan service MySQL di XAMPP / Laragon / OS Anda sudah dalam status **Running** dan database `db_wa_blast` sudah dibuat.
3. **Port 5173 atau 3030 sudah digunakan:**
   * Tutup terminal/aplikasi lain yang sedang memakai port tersebut, lalu jalankan kembali `npm run dev`.
