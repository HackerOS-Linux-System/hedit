# hedit

Terminalowy edytor tekstu w stylu `nano`, napisany w **H#** na bibliotece **tui**
(projekt zarządzany przez **bit**, manifest: `Bit.hk`).

To przepisana na H# wersja edytora, który wcześniej był napisany w Go
(bubbletea + chroma + clipboard). Stary kod został w `source-code/` bez zmian,
jako odniesienie – można go usunąć.

## Wymagania

- `h#` 0.9 lub nowszy oraz `bit` (`bit --version`)
- `g++` (backend C++ kompilatora H#)
- terminal z kolorami 24-bit (truecolor)
- opcjonalnie narzędzie schowka: `wl-copy`/`wl-paste`, `xclip`, `xsel`, `pbcopy`/`pbpaste` albo `termux-clipboard-*`

## Budowanie i uruchamianie

```sh
bit check            # sprawdza typy wszystkich modułów
bit test             # testy jednostkowe (src/selftest.h#)
bit build --release  # buduje binarkę `hedit`
./hedit plik.txt
```

Zależność `tui` jest pobierana z indeksu bit (`-> tui => newest`). Do pracy offline
odkomentuj w `Bit.hk` wariant `path ../tui-main`.

## Opcje

| Opcja | Znaczenie |
|---|---|
| `-t`, `--theme <nazwa>` | motyw kolorów (domyślnie `monokai`) |
| `--list-themes` | lista motywów: monokai, dracula, nord, gruvbox, solarized-dark, github-dark, one-dark, github |
| `--tabs <n>` | szerokość tabulacji / wcięcia (domyślnie 4) |
| `--hard-tabs` | wstawiaj prawdziwe tabulatory zamiast spacji |
| `--no-backup` | nie zapisuj `plik.bak` przy zapisie |
| `--mouse` | włącz mysz: klik, przeciąganie (zaznaczanie), kółko |
| `-h`, `-V` | pomoc, wersja |

## Klawisze

| Klawisz | Akcja |
|---|---|
| `Ctrl+O` | zapisz (poprzednia wersja pliku trafia do `plik.bak`) |
| `Ctrl+X` | wyjdź (przy zmianach pyta: Y / N / Esc) |
| `Ctrl+W` | szukaj (Enter przy tej samej frazie = następne trafienie), `F3` – następne |
| `Ctrl+R` | zamień wszystko (najpierw fraza, potem tekst zastępczy) |
| `Ctrl+G` | idź do linii |
| `Ctrl+Z` / `Ctrl+Y` | cofnij / ponów |
| `Ctrl+K` | wytnij zaznaczenie albo całą linię |
| `Ctrl+P` lub `Alt+C` | kopiuj |
| `Ctrl+U` lub `Alt+V` | wklej |
| `Ctrl+D` | powiel linię |
| `Ctrl+A` | zaznacz wszystko |
| `Alt+/` | przełącz komentarz (w zaznaczonych liniach) |
| `Tab` / `Shift+Tab` | wcięcie / cofnięcie wcięcia (przy zaznaczeniu wielu linii – całego bloku) |
| `Shift+strzałki`, `Shift+Home/End/PgUp/PgDn` | zaznaczanie |
| `Ctrl+←/→` | słowo; `Ctrl+Home/End` – początek / koniec pliku |
| `Home` | pierwszy znak niebiały, ponownie – początek linii |
| `Alt+Backspace` | usuń słowo wstecz |
| `Esc` | zdejmij zaznaczenie i podświetlenie wyszukiwania |
| `F1` | pomoc |
| `Ctrl+C` | natychmiastowe wyjście (patrz niżej) |

## Co się zmieniło względem wersji w Go

| Go | H# |
|---|---|
| bubbletea / lipgloss | biblioteka `tui` (`run_with`, `style_*`, `textinput_*`, `statusbar`) |
| chroma (lexery + style) | `syntax.h#` + `theme.h#` – własny, lekki tokenizer i 8 palet |
| atotto/clipboard | `clip.h#` – te same narzędzia powłoki + schowek wewnętrzny jako zapasowy |
| `undo.go` (rodzaje akcji ręcznie) | `textbuf.h#` – jeden prymityw „zamień zakres", cofanie i ponawianie dla każdej zmiany |
| `go.mod` | `Bit.hk` |
| `go build` | `bit build` (`build.hl`) |

Rozszerzenia: wpisywanie znaków UTF-8, kolorowanie wielu języków, „smart case" i
podświetlanie trafień, cofanie wszystkich operacji (też cięcie wielu linii i zamiana),
wcięcia i komentarze dla bloku linii, wklejanie liniowe vs. w miejscu kursora,
pozycja kursora na stałe w pasku statusu, zachowanie końców linii CRLF, obsługa myszy,
bracketed paste (wklejony Enter nie dodaje wcięć), motywy, opcje wiersza poleceń.

## Ograniczenia i różnice

- **`Ctrl+C`** jest przechwytywane przez `tui` i kończy program bez pytania. Stare
  „pozycja kursora" pod `Ctrl+C` jest teraz zawsze widoczne w pasku statusu. Jeśli bufor
  miał niezapisane zmiany, `hedit` zapisuje je do `plik.hedit-recovery`.
- Terminal nie rozróżnia `Ctrl+/` ani `Ctrl+Shift+C/V`, dlatego komentarz to `Alt+/`,
  a kopiowanie/wklejanie systemowe to `Alt+C` / `Alt+V` (`Ctrl+P` / `Ctrl+U` też działają).
- Kolorowanie składni liczone jest osobno dla każdej linii (jak w wersji w Go), więc
  komentarz blokowy rozciągający się na kilka linii nie jest rozpoznawany w kolejnych liniach.
- `tui` czyta klawisze po jednym bajcie przez `dd`, więc wklejanie bardzo długich tekstów
  z terminala jest powolne; do dużych wstawek użyj `Ctrl+U` (schowek).
- Szerokość znaków: CJK i emoji = 2 komórki, znaki łączące = 0 (uproszczone `wcwidth`).

## Układ projektu

```
Bit.hk            manifest bit (pakiet, zależność tui, komendy)
build.hl          budowanie
src/main.h#       argumenty, pętla tui
src/model.h#      stan edytora, wczytywanie i zapis
src/textbuf.h#    jedyne miejsce zmieniające tekst + cofanie/ponawianie
src/edit.h#       ruch kursora, wstawianie/usuwanie, wcięcia, komentarze, schowek
src/search.h#     szukanie i zamiana
src/syntax.h#     rozpoznawanie języka i tokenizer
src/theme.h#      palety kolorów
src/view.h#       rysowanie ekranu
src/update.h#     obsługa klawiszy, myszy i trybów (zapytania, wyjście)
src/clip.h#       schowek systemowy
src/util.h#       UTF-8, szerokość znaków, wyszukiwanie w napisach
src/selftest.h#   testy jednostkowe (#[test])
source-code/      oryginalna wersja w Go (bez zmian)
```

## Status

Kod został napisany bez dostępu do kompilatora H# w środowisku, w którym powstał,
więc nie był jeszcze skompilowany ani uruchomiony. Zacznij od `bit check`, potem
`bit test`; ewentualne błędy typów będą w pojedynczych funkcjach i powinny być proste
do poprawienia.

## Licencja

Apache-2.0 (plik `LICENSE`).
