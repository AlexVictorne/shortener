# go-musthave-shortener-tpl

Шаблон репозитория для трека «Сервис сокращения URL».

## Начало работы

1. Склонируйте репозиторий в любую подходящую директорию на вашем компьютере.
2. В корне репозитория выполните команду `go mod init <name>` (где `<name>` — адрес вашего репозитория на GitHub без префикса `https://`) для создания модуля.

## Обновление шаблона

Чтобы иметь возможность получать обновления автотестов и других частей шаблона, выполните команду:

```
git remote add -m v2 template https://github.com/Yandex-Practicum/go-musthave-shortener-tpl.git
```

Для обновления кода автотестов выполните команду:

```
git fetch template && git checkout template/v2 .github
```

Затем добавьте полученные изменения в свой репозиторий.

## Запуск автотестов

Для успешного запуска автотестов называйте ветки `iter<number>`, где `<number>` — порядковый номер инкремента. Например, в ветке с названием `iter4` запустятся автотесты для инкрементов с первого по четвёртый.

При мёрже ветки с инкрементом в основную ветку `main` будут запускаться все автотесты.

Подробнее про локальный и автоматический запуск читайте в [README автотестов](https://github.com/Yandex-Practicum/go-autotests).

## Структура проекта

Приведённая в этом репозитории структура проекта является рекомендуемой, но не обязательной.

Это лишь пример организации кода, который поможет вам в реализации сервиса.

При необходимости можно вносить изменения в структуру проекта, использовать любые библиотеки и предпочитаемые структурные паттерны организации кода приложения, например:
- **DDD** (Domain-Driven Design)
- **Clean Architecture**
- **Hexagonal Architecture**
- **Layered Architecture**

## Профилирование и оптимизация

### Запуск профилировщика

Сервер экспонирует pprof на `127.0.0.1:6060` (только локально, не на production-порту):

```bash
# Запуск с аудитом
go run ./cmd/shortener --audit-file=profiles/audit.log

# Предзаполнение хранилища
bash scripts/seed.sh http://127.0.0.1:8080 10000

# Нагрузочное тестирование
ab -n 50000 -c 100 http://127.0.0.1:8080/<short-id>

# Снятие профиля
curl -s http://127.0.0.1:6060/debug/pprof/heap > profiles/base.pprof

# Анализ
go tool pprof -alloc_space profiles/base.pprof
# Команды внутри: top, list FileAuditor.Emit, peek MemStorage
```

### Оптимизации

| Компонент | Проблема | Исправление |
|-----------|----------|-------------|
| `FileAuditor` | `os.OpenFile` + `f.Close()` на каждый `Emit()` | Открывается один раз в `NewFileAuditor`, запись через `bufio.Writer` (64 KB), `Close()` сбрасывает буфер |
| `MemStorage` | `sync.Mutex` сериализует параллельные читатели | `sync.RWMutex`: `Get`, `GetByOriginal`, `GetByUserID`, `ExportAll` используют `RLock` — читатели не блокируют друг друга |
| `TrimmerService` | `validateURL` + `normalizeURL` — двойной `url.Parse` на каждый запрос; `buildShortURL` вызывает `url.JoinPath` (→ `path.Join` + `path.Clean` + `strings.Builder`) | Объединены в `validateAndNormalize` — один `url.Parse`; `basePrefix` вычисляется в конструкторе, `buildShortURL` = простая конкатенация |

### Результат: `pprof -alloc_space -top -diff_base=profiles/base.pprof profiles/result.pprof`

```
File: shortener-bin-result
Type: alloc_space
Time: 2026-06-27 02:52:26 +04
Showing nodes accounting for 16.50MB, 5.45% of 302.61MB total
Dropped 3 nodes (cum <= 1.51MB)
      flat  flat%   sum%        cum   cum%
   -6.50MB  2.15%  2.15%    -6.50MB  2.15%  net/http.Header.Clone (inline)
    6.02MB  1.99%  0.16%     6.02MB  1.99%  bufio.NewReaderSize (inline)
       6MB  1.98%  1.82%        6MB  1.98%  context.(*cancelCtx).Done
      -6MB  1.98%  0.16%       -6MB  1.98%  os.newFile
    5.52MB  1.82%  1.67%     5.52MB  1.82%  bufio.NewWriterSize (inline)
   -5.50MB  1.82%  0.15%    -5.50MB  1.82%  net/http.(*Server).newConn (inline)
       5MB  1.65%  1.50%        5MB  1.65%  net/url.parse
    4.50MB  1.49%  2.99%    10.50MB  3.47%  context.(*cancelCtx).propagateCancel
    4.50MB  1.49%  4.47%    -3.50MB  1.16%  shortener/pkg/audit.(*FileAuditor).Emit
       4MB  1.32%  5.80%        4MB  1.32%  net/http.(*Request).SetPathValue (inline)
   -3.50MB  1.16%  4.64%    -3.50MB  1.16%  net.sockaddrToTCP
      -2MB  0.66%  3.81%    -1.50MB   0.5%  encoding/json.Marshal
      -2MB  0.66%  3.15%    -5.50MB  1.82%  net/http.Redirect
         0     0%  5.45%    -6.50MB  2.15%  os.OpenFile
         0     0%  5.45%    -6.50MB  2.15%  os.openFileNolog
         0     0%  5.45%    -3.50MB  1.16%  shortener/internal/handler.(*Handler).emitAudit
         0     0%  5.45%    -8MB     2.64%  shortener/internal/handler.(*Handler).RedirectHandler
```

Ключевые отрицательные значения:
- **`os.newFile` −6 MB** — устранены аллокации `os.OpenFile` на каждый аудит-вызов
- **`FileAuditor.Emit` cumulative −3.5 MB** — меньше аллокаций внутри Emit
- **`os.OpenFile` / `os.openFileNolog` −6.5 MB** — файловый дескриптор больше не открывается заново
- **`encoding/json.Marshal` −2 MB** — суммарно меньший буфер аллокаций

### Benchstat: FileAuditor до и после

```
go test -bench=BenchmarkFileAuditor_Emit -benchmem -count=6 ./pkg/audit/ > old.txt  # оригинальный код
go test -bench=BenchmarkFileAuditor_Emit -benchmem -count=6 ./pkg/audit/ > new.txt  # после оптимизации
benchstat old.txt new.txt
```

```
                        │   old.txt    │              new.txt               │
                        │   sec/op     │   sec/op     vs base               │
FileAuditor_Emit-10       17.30µ ± 1%   1.274µ ± 1%  -92.63% (p=0.002 n=6)
FileAuditor_EmitParallel  18.96µ ± 1%   2.029µ ± 3%  -89.30% (p=0.002 n=6)

                        │   old.txt    │              new.txt               │
                        │    B/op      │    B/op      vs base               │
FileAuditor_Emit-10       376.0 ± 0%    160.0 ± 0%   -57.45% (p=0.002 n=6)
FileAuditor_EmitParallel  376.0 ± 0%    160.0 ± 0%   -57.45% (p=0.002 n=6)

                        │   old.txt    │              new.txt               │
                        │  allocs/op   │  allocs/op   vs base               │
FileAuditor_Emit-10        5.000 ± 0%   2.000 ± 0%   -60.00% (p=0.002 n=6)
FileAuditor_EmitParallel   5.000 ± 0%   2.000 ± 0%   -60.00% (p=0.002 n=6)
```

**Результат**: −92% латентность, −57% память на операцию, −60% аллокаций. Основная экономия — устранение `os.OpenFile` (3 аллокации) и `f.Close()` на каждый вызов.

### Benchstat: MemStorage параллельные чтения

```
go test -bench='Parallel' -benchmem -count=6 ./internal/repository/ > mutex.txt   # sync.Mutex
# применить RWMutex
go test -bench='Parallel' -benchmem -count=6 ./internal/repository/ > rwmutex.txt
benchstat mutex.txt rwmutex.txt
```

```
                                    │  mutex.txt  │          rwmutex.txt               │
                                    │   sec/op    │   sec/op     vs base               │
MemStorage_GetParallel-10             153.4n ± 15%  108.0n ± 1%  -29.60% (p=0.002 n=6)
MemStorage_GetByOriginalParallel-10   215.8n ±  2%  111.8n ± 1%  -48.23% (p=0.002 n=6)
geomean                               182.0n        109.9n       -39.63%
```

**Результат**: −29–48% latency параллельно

### Benchstat: TrimmerService

```
go test -bench=BenchmarkTrimURL -benchmem -count=6 ./internal/service/ > trim_old.txt  # оригинал
# применить оптимизации
go test -bench=BenchmarkTrimURL -benchmem -count=6 ./internal/service/ > trim_new.txt
benchstat trim_old.txt trim_new.txt
```

```
           │ trim_old.txt │         trim_new.txt               │
           │   sec/op     │   sec/op     vs base               │
TrimURL-10    1304.0n ± 5%   714.8n ± 2%  -45.18% (p=0.002 n=6)

           │   B/op       │    B/op     vs base                │
TrimURL-10    1239.0 ± 5%   620.0 ± 1%   -49.96% (p=0.002 n=6)

           │  allocs/op   │  allocs/op  vs base                │
TrimURL-10    14.000 ± 0%   7.000 ± 0%   -50.00% (p=0.002 n=6)
```

**Результат**: −45% latency, −50% памяти, −50% аллокаций (14→7). Устранены двойной `url.Parse` и аллокации `url.JoinPath`.

### Базовые замеры: pkg/auth и pkg/generator

Оптимизаций не вносилось — компоненты уже эффективны, профилировщик их не выявил как узкие места.

```bash
go test -bench=. -benchmem -count=6 ./pkg/auth/ ./pkg/generator/
```

```
BenchmarkSign-10          291 ns/op    816 B/op   11 allocs/op
BenchmarkVerify-10        310 ns/op    704 B/op   10 allocs/op
BenchmarkGenerateID-10     68 ns/op     16 B/op    1 allocs/op
```

- `GenerateID` — 1 аллокация, 68 ns: читает `crypto/rand` в срез и кодирует base64 на месте
- `Sign` / `Verify` — HMAC-SHA256 + hex encoding, стабильны (11/10 allocs), вызываются только при аутентификации

### Нагрузочное тестирование (ab)

```bash
# До
ab -n 50000 -c 100 http://127.0.0.1:8080/<short-id>
# rps: 35685.23 [#/sec]

# После
ab -n 50000 -c 100 http://127.0.0.1:8080/<short-id>
# rps: 48444.71 [#/sec]
```

**Результат**: 35 685 → 48 444 rps (**+35%**)
