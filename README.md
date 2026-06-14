# Restaurant POS Backend

Backend REST API dan WebSocket untuk aplikasi POS restoran. Sistem mendukung pemesanan customer melalui QR meja atau QR takeaway, input manual kasir, konfirmasi pembayaran, kitchen display, upload gambar lokal, dan laporan penjualan admin.

## Tech Stack

- Go 1.24
- Fiber v2
- PostgreSQL
- GORM
- JWT Authentication
- bcrypt password hashing
- Fiber WebSocket

## Install Dependency

```powershell
cd backend
go mod tidy
```

## Setup PostgreSQL

Buat database PostgreSQL:

```sql
CREATE DATABASE restaurant_pos;
```

Pastikan user PostgreSQL memiliki akses ke database tersebut. Schema tabel dibuat otomatis melalui GORM `AutoMigrate` saat aplikasi dijalankan.

## Environment

Salin `.env.example` menjadi `.env`, lalu sesuaikan kredensial PostgreSQL dan JWT secret:

```env
APP_PORT=8080
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=
DB_NAME=restaurant_pos
JWT_SECRET=your_secret_key
FRONTEND_URL=http://localhost:3000
RUN_SEEDER=true
MIDTRANS_SERVER_KEY=
MIDTRANS_CLIENT_KEY=
MIDTRANS_IS_PRODUCTION=false
FRONTEND_PAYMENT_FINISH_URL=http://localhost:3000/payment/finish
```

Gunakan `RUN_SEEDER=true` untuk membuat data awal secara otomatis saat startup. Seeder bersifat idempoten dan tidak membuat duplikat jika aplikasi dijalankan berkali-kali. Setelah bootstrap awal, nilainya dapat diubah menjadi `false`.

## Menjalankan Backend

```powershell
go run .
```

Backend berjalan di `http://localhost:8080`. Health check tersedia di:

```text
GET http://localhost:8080/api/health
```

File gambar tersimpan di `uploads/` dan dapat diakses melalui:

```text
http://localhost:8080/uploads/menus/<filename>
http://localhost:8080/uploads/payments/<filename>
```

## Akun Default

| Role | Email | Password |
|---|---|---|
| Admin | `admin@mail.com` | `admin123` |
| Cashier | `cashier@mail.com` | `cashier123` |
| Kitchen | `kitchen@mail.com` | `kitchen123` |

Ganti password default sebelum dipakai pada lingkungan produksi.

## Endpoint Utama

### Auth

```text
POST /api/auth/register
POST /api/auth/login
GET  /api/auth/me
```

### Categories

```text
GET    /api/categories
GET    /api/categories/:id
POST   /api/categories
PUT    /api/categories/:id
DELETE /api/categories/:id
```

### Users

```text
GET    /api/users
GET    /api/users/:id
POST   /api/users
PUT    /api/users/:id
DELETE /api/users/:id
```

### Menus

```text
GET   /api/menus
GET   /api/menus/:id
GET   /api/menus/category/:category_id
POST  /api/menus
PUT   /api/menus/:id
DELETE /api/menus/:id
PATCH /api/menus/:id/availability
POST  /api/admin/menus/:id/upload-image
```

### Tables

```text
GET    /api/tables
GET    /api/tables/:id
POST   /api/tables
PUT    /api/tables/:id
DELETE /api/tables/:id
PATCH  /api/tables/:id/status
```

### QR Codes

```text
GET   /api/qrcodes
GET   /api/qrcodes/:id
POST  /api/qrcodes/generate-table/:table_id
POST  /api/qrcodes/generate-takeaway
PATCH /api/qrcodes/:id/active
```

### Public Customer

```text
GET  /api/public/menu
GET  /api/public/menu/:id
GET  /api/public/qrcode/:code
POST /api/public/orders
GET  /api/public/orders/:order_code
POST /api/public/orders/:order_code/upload-payment-proof
POST /api/public/orders/:order_code/upload-payment-proof-file
```

### Cashier

```text
POST  /api/cashier/orders
GET   /api/cashier/orders
GET   /api/cashier/orders/:id
GET   /api/cashier/orders/:id/payment-status
POST  /api/cashier/orders/:id/payment/retry
PATCH /api/cashier/orders/:id/confirm-payment
PATCH /api/cashier/orders/:id/cancel
PATCH /api/cashier/orders/:id/complete
GET   /api/cashier/payments/waiting-confirmation
```

### Midtrans Snap

```text
POST /api/payments/midtrans/create-snap/:order_id
POST /api/payments/midtrans/notification
POST /api/payments/midtrans/sync/:order_code
GET  /api/payments/midtrans/status/:order_code
```

`POST /api/payments/midtrans/create-snap/:order_id` membutuhkan JWT role `cashier` atau `admin`. Endpoint ini dipanggil setelah order dibuat untuk payment method `qris`, lalu mengembalikan `snap_token` dan `redirect_url` untuk frontend.

`POST /api/payments/midtrans/notification` adalah webhook public dari Midtrans. Untuk testing localhost, gunakan ngrok atau deploy backend agar URL webhook dapat diakses dari dashboard Midtrans Sandbox. Gunakan `MIDTRANS_IS_PRODUCTION=false` untuk Sandbox.

`POST /api/payments/midtrans/sync/:order_code` membutuhkan JWT role `cashier` atau `admin`. Endpoint ini berguna untuk development lokal saat webhook tidak bisa dipakai: backend akan mengecek status transaksi ke Midtrans Transaction Status API, lalu menyinkronkan status order dan payment.

`GET /api/cashier/orders/:id/payment-status` membutuhkan JWT role `cashier` atau `admin`. Endpoint ini dipakai frontend kasir setelah customer menyelesaikan Snap: backend akan mengecek status terbaru ke Midtrans untuk order online yang punya Snap/Midtrans reference, menyimpan status sukses, lalu mengembalikan data order terbaru.

`POST /api/cashier/orders/:id/payment/retry` membutuhkan JWT role `cashier` atau `admin`. Endpoint ini hanya untuk order `qris` yang belum paid; backend akan memakai `snap_token` lama jika masih pending, atau membuat transaksi Snap baru untuk retry jika transaksi lama sudah final/gagal.

Manual confirm payment tetap tersedia melalui:

```text
PATCH /api/cashier/orders/:id/confirm-payment
```

### Kitchen

```text
GET   /api/kitchen/orders
PATCH /api/kitchen/orders/:id/cooking
PATCH /api/kitchen/orders/:id/ready
```

### Admin Reports

```text
GET /api/admin/reports/sales
GET /api/admin/reports/sales?start_date=2026-01-01&end_date=2026-01-31
```

### WebSocket

```text
GET /ws/cashier?token=<jwt>
GET /ws/kitchen?token=<jwt>
GET /ws/customer/:order_code
```

Channel cashier dan kitchen memakai JWT. Channel customer bersifat public tetapi hanya menerima update untuk `order_code` terkait.

## Flow Testing

Gunakan [docs/api-test.http](docs/api-test.http) melalui REST Client VS Code atau JetBrains HTTP Client.

1. Login admin, cashier, dan kitchen untuk mendapatkan JWT.
2. Admin dapat membuat kategori atau menu baru.
3. Admin dapat membuat QR meja tambahan melalui `POST /api/qrcodes/generate-table/:table_id`.
4. Customer melihat menu public dan membuat order dine-in dari QR.
5. Customer mengirim bukti pembayaran melalui URL string atau upload file.
6. Cashier melihat antrean payment dan mengonfirmasi pembayaran.
7. Kitchen mengubah order menjadi `cooking`.
8. Kitchen mengubah order menjadi `ready`.
9. Cashier menandai order sebagai `completed`.

Untuk frontend realtime, buka WebSocket sebelum menjalankan flow status. Event yang tersedia:

```text
order_created
payment_waiting_confirmation
payment_confirmed
order_sent_to_kitchen
order_cooking
order_ready
order_completed
order_cancelled
```
