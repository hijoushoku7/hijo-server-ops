# サードパーティライセンス

hso のバイナリには以下の Go モジュールが静的リンクされている。
同じ本文のライセンスはまとめ、著作権表示を原文のまま列挙する。

このファイルは `./scripts/gen-third-party-licenses.sh` で生成する。

加えて Go 標準ライブラリとランタイム (BSD-3-Clause, Copyright 2009 The Go Authors)
も同梱される。全文は https://go.dev/LICENSE を参照。

## MIT License

- `charm.land/bubbletea/v2@v2.0.8` — Copyright (c) 2020-2026 Charmbracelet, Inc.
- `charm.land/lipgloss/v2@v2.0.5` — Copyright (c) 2021-2026 Charmbracelet, Inc.
- `github.com/BurntSushi/toml@v1.5.0` — Copyright (c) 2013 TOML authors
- `github.com/charmbracelet/colorprofile@v0.4.3` — Copyright (c) 2020-2024 Charmbracelet, Inc
- `github.com/charmbracelet/ultraviolet@v0.0.0-20260703014108-f5a850f9c2b7` — Copyright (c) 2025 Charmbracelet, Inc
- `github.com/charmbracelet/x/ansi@v0.11.7` — Copyright (c) 2023 Charmbracelet, Inc.
- `github.com/charmbracelet/x/termios@v0.1.1` — Copyright (c) 2023 Charmbracelet, Inc.
- `github.com/charmbracelet/x/term@v0.2.2` — Copyright (c) 2023 Charmbracelet, Inc.
- `github.com/charmbracelet/x/windows@v0.2.2` — Copyright (c) 2023 Charmbracelet, Inc.
- `github.com/clipperhouse/displaywidth@v0.11.0` — Copyright (c) 2025 Matt Sherman
- `github.com/clipperhouse/uax29/v2@v2.7.0` — Copyright (c) 2020 Matt Sherman
- `github.com/lucasb-eyer/go-colorful@v1.4.0` — Copyright (c) 2013 Lucas Beyer
- `github.com/mattn/go-runewidth@v0.0.23` — Copyright (c) 2016 Yasuhiro Matsumoto
- `github.com/muesli/cancelreader@v0.2.2` — Copyright (c) 2022 Erik Geiser and Christian Muehlhaeuser
- `github.com/rivo/uniseg@v0.4.7` — Copyright (c) 2019 Oliver Kuederle
- `github.com/xo/terminfo@v0.0.0-20220910002029-abceb7e1c41e` — Copyright (c) 2016 Anmol Sethi

```
Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## BSD 3-Clause License

- `golang.org/x/sync@v0.21.0` — Copyright 2009 The Go Authors.
- `golang.org/x/sys@v0.46.0` — Copyright 2009 The Go Authors.

```
Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google LLC nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```
