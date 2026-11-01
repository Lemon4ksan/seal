<img src="assets/logo.svg" align="right" width="128" height="128" alt="seal logo" />

# seal

_«Энтропия разрушает любые данные — пусть eon сохранит реальность навечно»_

[![Go Version](https://img.shields.io/badge/go-1.27%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/seal)
[![Architecture](https://img.shields.io/badge/arch-EON%20%7C%20Reed--Solomon%20%7C%20LBA-0d9488?style=flat-square)](#правила)
[![Ecosystem](https://img.shields.io/badge/ecosystem-foundation%20%7C%20ctrl-0284c7?style=flat-square)](https://github.com/lemon4ksan/foundation)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)

**[English](README.md)** | **[Русский](README_RU.md)** | **[Спецификация формата EON](docs/EON_SPEC.md)**

Отказоустойчивый движок архивации и формат контейнеров для Go 1.27+. Реализует контейнерный формат **EON (`.eon`)** наряду с `.7z`, `.zip`, `.tar` и `.zst`, объединяя помехоустойчивое кодирование Коши — Рида — Соломона над полем $GF(2^8)$, $O(1)$ LBA-поиск страниц, детерминированный потолок памяти $\le 32\text{ МБ}$ и блочную дедупликацию по BLAKE3-256.

## Инженерные компромиссы

| `archive/zip` / `archive/tar` / монолитный `.7z` | Что делает `seal` (`.eon`) | Цена решения |
| :--- | :--- | :--- |
| Архивы `.tar.gz` и непрерывные `.7z` требуют линейного сканирования или распаковки гигабайт данных ради чтения одного файла | Хранит 32-байтный футер и сжатое оглавление (`CTOC`) с выравниванием по страницам LBA $2^{\text{BlockSizeShift}}$ (4 КБ) для произвольного чтения за $<2\text{ мс}$ и `mmap` | Выравнивание смещений по 4 КБ LBA добавляет небольшие паддинги между независимыми solid-чанками |
| Большие словари LZMA2 могут требовать $1\text{–}4\text{ ГБ}$ ОЗУ при распаковке, вызывая `OOMKilled` в контейнерах | Разбивает данные на независимые solid-чанки размером не более $\le 32\text{ МБ}$ (с суб-чанковым микро-фреймингом до $\le 64\text{ КБ}$ ОЗУ) | Ограничение размера solid-чанка в 32 МБ уступает ~1–2% степени сжатия на многогигабайтных однородных дампах по сравнению с безлимитным монолитным словарём |
| Один повреждённый бит или битый сектор разрушает весь сжатый поток | Вычисляет блоки восстановления Коши — Рида — Соломона над $GF(2^8)$ и проверяет экстенты по дереву Меркла BLAKE3-256 | Блоки чётности Рида — Соломона при включении требуют дополнительного места на диске и тактов CPU на кодирование |
| Стандартные распаковщики доверяют путям файлов и заголовкам размера в архиве | По умолчанию блокирует обход путей Zip Slip (`../`) и ограничивает коэффициент расширения бомб декомпрессии | Неканоничные пути, выходящие за корень распаковки, отклоняются, если они не переименованы явно через VFS |

## Правила

1. **Детерминированный потолок ОЗУ.** Упаковка и распаковка никогда не превышают заданный лимит чанка ($\le 32\text{ МБ}$ по умолчанию), исключая неконтролируемый рост кучи на недоверенных архивах.
2. **Ноль аллокаций при потоковом чтении.** Блочные ридеры, верификаторы контрольных сумм и итераторы VFS переиспользуют буферы из пула (`0 B/op` на 64 КБ блоках).
3. **Безопасность распаковки по умолчанию.** Каждая операция извлечения проверяет пути на Zip Slip (`../`), выход симлинков за корень и аномальные коэффициенты сжатия до обращения к диску хоста.
4. **Контентно-адресуемая дедупликация.** Одинаковые файлы (совпадающие по BLAKE3-256) ссылаются на существующий экстент данных без повторного сжатия и дублирования на диске.

## Структура

- [`seal` (корень)](.) — Высокоуровневые функции `Compress`, `Extract`, `ReadFile`, параметризованный `ReadTo[T]`, `Verify`, `Tree`, `NewArchive` и предикаты ошибок (`IsNotFound`, `IsZipSlip`, `IsCorrupted`).
- [`archive`](archive) — Кодеки форматов (`EON`, `7z` / `LZMA2`, `ZIP`, `TAR`), коды восстановления Рида — Соломона над $GF(2^8)$, деревья целостности BLAKE3-256, ЭЦП Ed25519 в TLV-полях и шифрование XChaCha20-Poly1305 / AES-256-GCM.
- [`vfs`](vfs) — Виртуальная файловая система в памяти (`io/fs.FS`) для сборки, модификации, сортировки и потоковой отдачи архивов без временных файлов на диске.
- [`cmd/seal`](cmd/seal) — CLI-утилита на базе [`ctrl`](https://github.com/lemon4ksan/ctrl) (`create`, `extract`, `list`, `cat`, `test`, `tree`).
- [`docs/EON_SPEC.md`](docs/EON_SPEC.md) — Спецификация бинарного формата `.eon`, LBA-выравнивания, CTOC и кодов восстановления.

## Установка

```bash
go get github.com/lemon4ksan/seal
```

## Использование

### Упаковка, безопасная распаковка и чтение одного файла за $O(1)$

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/lemon4ksan/seal"
)

type AppConfig struct {
	Port int `json:"port"`
}

func main() {
	ctx := context.Background()

	// Упаковка директории (формат .eon, .zip, .tar.gz, .7z определяется по расширению)
	if err := seal.Compress(ctx, "./dist", "release.eon"); err != nil {
		log.Fatal(err)
	}

	// Чтение и парсинг одного файла за O(1) без распаковки всего архива
	cfg, err := seal.ReadTo[AppConfig](ctx, "release.eon", "configs/app.json")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Port: %d\n", cfg.Port)

	// Безопасная распаковка с защитой от Zip Slip и бомб декомпрессии
	if err := seal.Extract(ctx, "release.eon", "./output"); err != nil {
		log.Fatal(err)
	}
}
```

### Виртуальный архив в ОЗУ (`vfs` / `io/fs.FS`)

```go
archive := seal.NewArchive(
	seal.WithContainer(container.Zip),
	seal.WithCompression(seal.Deflate, 9),
	seal.WithPassword("passphrase123"),
	seal.WithSortStrategy(seal.SortNameAsc),
)

archive.AddBytes([]byte(`{"status":"ok"}`), "meta/health.json")
archive.AddString("v1.0.0", "VERSION")
archive.Rename("meta/health.json", "status.json")

// Использование напрямую как стандартной io/fs.FS или потоковая запись в io.Writer
file, _ := archive.Open("status.json")
defer file.Close()
```

### CLI-утилита

```bash
# Упаковать директорию в контейнер .eon
seal create -o bundle.eon -l 9 ./dist

# Просмотреть список файлов, коэффициенты сжатия и дедуплицированные экстенты
seal list bundle.eon

# Извлечь один файл напрямую в stdout за время O(1)
seal cat bundle.eon configs/app.json

# Проверить дерево BLAKE3-256 и восстановить повреждённые блоки по кодам Рида-Соломона
seal test bundle.eon
```

## Бенчмарки

Замеры на 12 потоках AMD64 (Go 1.27) на смешанном наборе исходников/бинарников и потоковых блоках по 64 КБ:

| Сценарий | Метрика | `seal` | Базовый ориентир |
| :--- | :--- | ---: | ---: |
| `.7z` / LZMA2 Уровень 1 (12T) | Сжатие / Распаковка | `136.1 MB/s` / `227.6 MB/s` | `51.7 MB/s` / `72.7 MB/s` (7-Zip CLI) |
| `.7z` / LZMA2 Уровень 5 (12T) | Сжатие / Распаковка | `158.3 MB/s` / `194.2 MB/s` | `18.4 MB/s` / `69.9 MB/s` (7-Zip CLI) |
| `.7z` / LZMA2 Уровень 9 (12T) | Сжатие / Распаковка | `116.1 MB/s` / `240.2 MB/s` | `8.3 MB/s` / `46.1 MB/s` (7-Zip CLI) |
| Потоковое чтение/запись (64 КБ) | Скорость / Аллокации | `1,820 MB/s` (`0 B/op`, `0 allocs`) | `680 MB/s` (`1,420 B/op`, `archive/zip`) |

## Лицензия

BSD 3-Clause License. Полный текст приведён в файле [LICENSE](LICENSE).
