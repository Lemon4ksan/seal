<div align="center">

# seal

### Универсальный отказоустойчивый движок архивации и сжатия для Go

_«В хранилищах энтропия — норма. Пусть seal сожмет реальность до предела.»_

[![Go Version](https://img.shields.io/badge/go-1.27%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/seal)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)
[![Zero-Alloc](https://img.shields.io/badge/memory-0%20B%2Fop%20%7C%200%20allocs-brightgreen?style=flat-square)](#-производительность)
[![Security](https://img.shields.io/badge/security-Zip%20Slip%20%26%20Bomb%20Proof-success?style=flat-square)](#безопасность-по-умолчанию)

**seal** — это сверхбыстрый lock-free движок архивации и субстрат сжатия для Go 1.27+. Он объединяет форматы ZIP, TAR, GZIP и шифрование WinZip AES-256 с автоматическим спасением битых данных, виртуальными файловыми системами в памяти (`io/fs`) и встроенной защитой от Zip-Slip.

#### [English](README.md) • Русский • [Архитектура](archive) • [Виртуальная FS](vfs) • [Бенчмарки](codec)

</div>

## Почему seal?

Стандартные библиотеки Go (`archive/zip`, `archive/tar`) многословны, уязвимы к атакам выхода за пределы директории (Zip Slip) и полностью ломаются при малейшем повреждении заголовков архива или отсутствии Central Directory.

* **Простота в одну строку:** Упаковка или распаковка директорий, файлов и потоков в 1 строчку кода.
* **Спасение поврежденных архивов:** Восстанавливает и извлекает неповрежденные файлы из битых, усеченных архивов или архивов без заголовков.
* **Безопасность по умолчанию:** Встроенная защита от Zip Slip (`../` path traversal) и декомпрессионных бомб (`42.zip`).
* **100% Lock-Free параллелизм:** Параллельное чтение и инспекция архивов между горутинами с атомарными метаданными (Copy-On-Write).
* **Zero-Alloc VFS в памяти:** Создание, изменение, сортировка и монтирование виртуальных архивов прямо в RAM как стандартной `io/fs.FS`.

## Установка

```bash
go get github.com/lemon4ksan/seal
```

## Быстрый старт

### 1. Упаковка и распаковка в одну строку

```go
package main

import (
	"log"

	"github.com/lemon4ksan/seal"
)

func main() {
	// Упаковать директорию в оптимизированный ZIP (или .tar.gz)
	if err := seal.ArchiveDir("./dist", seal.ToFilePath("release.zip")); err != nil {
		log.Fatal(err)
	}

	// Распаковать безопасно (защита Zip Slip активна по умолчанию)
	if err := seal.Unarchive(seal.FromFilePath("release.zip"), "./output"); err != nil {
		log.Fatal(err)
	}
}
```

### 2. Повседневные операции

```go
// Автоматическое определение формата (.zip, .tar, .tar.gz) и распаковка
_ = seal.Unarchive(seal.FromFilePath("bundle.tar.gz"), "./extracted")

// Чтение одного файла напрямую в память без распаковки всего архива
data, err := seal.ReadFile(seal.FromFilePath("bundle.zip"), "configs/app.json")

// Проверка целостности CRC-32 по всем записям
err = seal.Verify(seal.FromFilePath("bundle.zip"))

// Отрисовка дерева иерархии архива в виде ASCII
tree, _ := seal.Tree(seal.FromFilePath("bundle.zip"))
fmt.Println(tree)
```

## Виртуальные архивы в памяти (VFS Builder)

Создание, мутация, шифрование и стриминг архивов в RAM без обращения к диску:

```go
// Создать виртуальный архив с шифрованием WinZip AES-256
archive := seal.NewArchive(
	seal.WithContainer(seal.FormatZip),
	seal.WithCompression(seal.Deflate, 9),
	seal.WithPassword("passphrase123"),
	seal.WithSortStrategy(seal.SortNameAsc),
)

// Добавление срезов байтов, строк или ленивых потоков
archive.AddBytes([]byte(`{"status":"ok"}`), "meta/health.json")
archive.AddString("v1.0.0", "VERSION")
archive.AddLazy("dump.sql", func() (io.ReadCloser, error) {
	return os.Open("/var/dump.sql")
})

// Монтирование стандартных файловых систем Go embed
archive.AddFS(embeddedAssets)

// Мутация виртуальной иерархии
archive.Rename("meta/health.json", "status.json")

// Использование напрямую как файловой системы io/fs.FS
file, err := archive.Open("status.json")

// Стриминг сжатых данных напрямую в HTTP response или на диск
_, err = archive.WriteTo(w)
```

## ⚡ Производительность и бенчмарки

### 7-Zip (.7z / LZMA2) против официального 7-Zip CLI (C/C++)
Тестирование на 12-поточном AMD64 (Go 1.27) на реальном многофайловом датасете (исходный код Go, бинарники, JSON):

| Уровень сжатия | Метрика | `seal` (Pure Go + SIMD) | 7-Zip Official (C/C++) | Превосходство |
| :--- | :--- | :---: | :---: | :---: |
| **Level 1** *(Fastest)* | Сжатие (12 потоков) | **56 мс** (136.1 MB/s) | 147 мс (51.7 MB/s) | **В 2.6x быстрее** |
| | Распаковка (12 потоков) | **34 мс** (227.6 MB/s) | 105 мс (72.7 MB/s) | **В 3.1x быстрее** |
| | Сжатие / Размер | 22.1% (1.72 MB) | 20.1% (1.57 MB) | Δ 2.0% |
| **Level 5** *(Normal)* | Сжатие (12 потоков) | **48 мс** (158.3 MB/s) | 415 мс (18.4 MB/s) | **В 8.6x быстрее** |
| | Распаковка (12 потоков) | **39 мс** (194.2 MB/s) | 109 мс (69.9 MB/s) | **В 2.8x быстрее** |
| | Сжатие / Размер | 22.1% (1.72 MB) | 21.0% (1.63 MB) | Δ 1.1% |
| **Level 9** *(Ultra)* | Сжатие (12 потоков) | **66 мс** (116.1 MB/s) | 918 мс (8.3 MB/s) | **В 13.9x быстрее** |
| | Распаковка (12 потоков) | **32 мс** (240.2 MB/s) | 165 мс (46.1 MB/s) | **В 5.2x быстрее** |
| | Сжатие / Размер | 22.0% (1.72 MB) | 20.7% (1.61 MB) | Δ 1.3% |

### Общая скорость архивации (ZIP / TAR / TGZ / ZST)

| Движок / Субстрат | Пропускная способность (64KB) | Аллокации | Память / оп | Форматы | Защита Zip Slip |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **`seal` (Zero-Alloc)** | **1.82 GB/s** | **0 allocs/op** | **0 B/op** | **7Z / ZIP / TAR / TGZ / ZST** | **✓ (Встроена)** |
| Go `archive/zip` | 680 MB/s | 14 allocs/op | 1,420 B/op | Только ZIP | ✗ (Уязвим) |
| Go `archive/tar` | 710 MB/s | 12 allocs/op | 1,180 B/op | Только TAR | ✗ (Уязвим) |
| `mholt/archiver/v4` | 520 MB/s | 38 allocs/op | 4,280 B/op | Мульти | ⚠️ (Вручную) |

## Безопасность по умолчанию

1. **Защита от Zip Slip Traversal:** Отклоняет любые записи, пытающиеся выйти за пределы целевой директории распаковки (`../../etc/passwd`).
2. **Щит от декомпрессионных бомб:** Постоянно отслеживает коэффициент распаковки и прерывает обработку, если вложенные бомбы (например, `42.zip`) превышают лимиты безопасности.
3. **Шифрование WinZip AES-256:** Деривация ключей PBKDF2-HMAC-SHA1 с потоковым шифрованием AES-CTR и 80-битными тегами аутентификации HMAC.
4. **PKWARE ZipCrypto:** Обратная совместимость для распаковки устаревших архивов.

## Обработка ошибок

Удобная проверка ошибок с помощью встроенных предикатов:

```go
data, err := seal.ReadFile(seal.FromFilePath("archive.zip"), "app.json")
if err != nil {
	switch {
	case seal.IsNotFound(err):
		// Запись не существует в архиве
	case seal.IsPasswordMismatch(err):
		// Неверный пароль или отсутствует ключ дешифрования
	case seal.IsZipSlip(err):
		// Обнаружена попытка path traversal
	case seal.IsResourceLimit(err):
		// Превышен лимит безопасности (zip-бомба)
	case seal.IsCorrupted(err):
		// Несовпадение CRC-32 или поврежденный фрейм файла
	}
}
```

## Консольная утилита (CLI)

`seal` включает в себя готовую утилиту командной строки:

```bash
# Сжать папку с шифрованием AES-256
seal create -o bundle.zip -p "pass123" -l 9 ./dist

# Безопасная распаковка
seal extract -d ./output bundle.zip

# Посмотреть записи и коэффициенты сжатия
seal list bundle.zip

# Вывести файл из архива прямо в stdout
seal cat bundle.zip config.json

# Проверить целостность и контрольные суммы CRC
seal test bundle.zip

# Отрисовать ASCII дерево архива в терминале
seal tree bundle.zip
```

## Лицензия

Лицензия BSD-3-Clause. Copyright (c) 2026 Lemon4ksan. Все права защищены.
