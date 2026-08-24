# Panduan Instalasi & Menjalankan ChatLoop

Panduan menjalankan **ChatLoop WhatsApp AI Assistant** secara lokal di macOS, Windows, dan Linux.

## 1. Prasyarat

| Software | Fungsi |
|---|---|
| Visual Studio Code | Editor kode |
| Go | Backend API |
| Node.js LTS | Frontend Vite |
| MySQL 8.x | Database |

Verifikasi:

```bash
go version
node -v
npm -v
```

## 2. Siapkan project

Clone repository lalu buka folder project di VS Code.

```bash
git clone <repository-url>
cd ngirimwa
```

Buat `.env` dari contoh:

```bash
npm run setup:env
```

Sesuaikan konfigurasi database dan secret lokal di `.env`:

```ini
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=root
DB_PASS=
DB_NAME=db_wa_blast

JWT_SECRET=ganti_dengan_string_acak_minimal_32_karakter
SUPERADMIN_USERNAME=superadmin
SUPERADMIN_PASSWORD=ubah_password_ini
```

Jangan memasukkan `.env` ke Git.

## 3. Siapkan database

Buat database MySQL:

```sql
CREATE DATABASE db_wa_blast;
```

Nama database dapat diganti sesuai kebutuhan dan harus sama dengan `DB_NAME`.

## 4. Install dependency

Dari root project:

```bash
npm run setup
```

Perintah ini mengunduh dependency Go dan dependency frontend.

## 5. Jalankan aplikasi

```bash
npm run dev
```

Development server:

- Frontend Vite: `http://127.0.0.1:5173`
- Backend API: `http://127.0.0.1:3030`

Launcher akan menjalankan Vite dan backend Go dengan hot reload bila Air tersedia.

## 6. Login pertama

Buka:

```text
http://localhost:5173
```

Gunakan akun superadmin yang dikonfigurasi pada `.env` atau dibuat oleh mekanisme seed aplikasi. Setelah login, hubungkan nomor WhatsApp melalui menu koneksi WhatsApp dan ikuti alur QR/pairing yang tersedia.

## 7. Troubleshooting

### `go` atau `npm` tidak ditemukan

Pastikan Go dan Node.js sudah terpasang dan PATH sudah tersedia pada terminal baru.

### Database connection refused

Pastikan MySQL sedang berjalan dan nilai `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, dan `DB_NAME` benar.

### Port 5173 atau 3030 sudah digunakan

Hentikan proses yang memakai port tersebut lalu jalankan kembali `npm run dev`.

### Frontend tidak tampil setelah build

Pastikan `frontend/dist` sudah dibuat dan `STATIC_DIR` menunjuk ke lokasi build frontend yang benar pada deployment production.
