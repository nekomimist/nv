# Third-Party Notices

The Windows native-decode build (`make windows-native`, the `native_decode`
build tag on `GOOS=windows`) statically links third-party C code into the
resulting binary (`nv-native.exe`): the two image libraries below, plus the
GCC and MinGW-w64 runtimes noted at the end. The image libraries' licence
texts, taken verbatim from each project's `COPYING` file, are reproduced
below. Neither is linked into the default `nv`/`nv.exe`/`nv-native` (Linux)
builds; the Linux native-decode build links libpng, libjpeg-turbo, libwebp,
and libdeflate as *shared* libraries via `pkg-config` instead, which does
not require redistributing their licence text alongside the binary.

## libdeflate

Statically linked on Windows only, via `third_party/mingw` (see
`scripts/windows-deps.sh` and `internal/imgdecode/native_png_fastpath.go`).

License: MIT

```
Copyright 2016 Eric Biggers
Copyright 2024 Google LLC

Permission is hereby granted, free of charge, to any person
obtaining a copy of this software and associated documentation files
(the "Software"), to deal in the Software without restriction,
including without limitation the rights to use, copy, modify, merge,
publish, distribute, sublicense, and/or sell copies of the Software,
and to permit persons to whom the Software is furnished to do so,
subject to the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS
BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN
ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

Source: https://github.com/ebiggers/libdeflate (v1.24 `COPYING`)

## libwebp

Statically linked on Windows only, as `libwebpdecoder` (decode-only, no
encoder/`libsharpyuv`) via `third_party/mingw` (see
`scripts/windows-deps.sh` and `internal/imgdecode/native_webp.go`).

License: BSD-3-Clause

```
Copyright (c) 2010, Google Inc. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

  * Redistributions of source code must retain the above copyright
    notice, this list of conditions and the following disclaimer.

  * Redistributions in binary form must reproduce the above copyright
    notice, this list of conditions and the following disclaimer in
    the documentation and/or other materials provided with the
    distribution.

  * Neither the name of Google nor the names of its contributors may
    be used to endorse or promote products derived from this software
    without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

Source: https://github.com/webmproject/libwebp (v1.5.0 `COPYING`)

## GCC runtime libraries

The Windows binary is built with the MinGW-w64 GCC toolchain and links
`libgcc` and `libstdc++` statically (`-static-libgcc -static-libstdc++`,
see `internal/imgdecode/native_windows.go`), so that it depends only on
DLLs every Windows installation already has.

License: GPL-3.0-or-later WITH GCC-exception-3.1

Both libraries are covered by the GCC Runtime Library Exception version
3.1, which grants permission to combine them with independent modules and
convey the result "under terms of your choice", provided all target code
was produced by an Eligible Compilation Process. This build qualifies: the
whole toolchain — MinGW-w64 GCC, the Go toolchain, binutils, libdeflate and
libwebp — is GCC or other GPL-compatible software. The exception draws no
distinction between static and dynamic linking, and `libgcc` is linked into
every GCC-produced binary regardless.

Exception text: https://www.gnu.org/licenses/gcc-exception-3.1.html

The MinGW-w64 C runtime (`libmingw32`, `libmingwex`), also statically
linked, is distributed under the Zope Public License 2.1 with a number of
permissive per-file licences; see the `mingw-w64-common` package copyright
for the full breakdown.
