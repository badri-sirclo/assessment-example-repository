# weather-aggregator

Aplikasi Go yang mengambil data cuaca dari dua provider:
- **Open-Meteo** — implementasi lama, belum sempat di-refactor
- **WeatherAPI** — implementasi baru, sesuai standards

## Struktur

```
├── main.go
├── .env.example
└── internal/
    ├── config/
    │   └── config.go           ← config dari env vars
    ├── openmeteo/
    │   └── client.go           ← kode LAMA: fmt.Println, no timeout, no retry
    └── weatherapi/
        ├── client.go           ← kode BARU: slog, timeout, retry, typed struct
        └── client_test.go
```

## Jalankan

```bash
cp .env.example .env
# isi WEATHERAPI_KEY

go run main.go
```

## Test

```bash
go test ./...
```

