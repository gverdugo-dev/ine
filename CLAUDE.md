# CLAUDE.md

`ine` es un cliente en Go de la API JSON del INE (Tempus3) y una CLI, `ine`, para
navegar su catálogo y descargar muchas tablas de golpe, también desde una TUI (`ine tui`).

## La API

- Base `https://servicios.ine.es/wstempus/js/{ES|EN}/{FUNCION}/{input}`, sin clave ni login.
- Funciones que usa el cliente: `OPERACIONES_DISPONIBLES`, `OPERACION`, `TABLAS_OPERACION`,
  `DATOS_TABLA` y `DATOS_SERIE`. Para cualquier otra, `Client.RawJSON`.
- CSV por tabla en dos rutas según la familia de la tabla: `jaxiT3/files/t/{es|en}/csv_bdsc/{id}.csv`
  para las de Tempus3 y `jaxi/files/tpx/{es|en}/csv_bdsc/{id}.csv` para las PC-Axis servidas por
  Tempus3 ("tpx"). `TableCSV` prueba la primera y, si responde 204, la segunda. Separador `;`,
  coma decimal y UTF-8 con BOM, aunque la cabecera diga ISO-8859-15.
- Errores: la API no devuelve JSON de error. Una tabla desconocida da 404 en HTML, una
  operación desconocida da 500 en HTML y un CSV desconocido da 204. Las tres son
  `ErrNotFound` y no se reintentan; los 429 y demás 5xx sí, con backoff.
- **Restricción de volumen**: con algunas tablas grandes y un filtro de periodos (`nult`,
  `date`), la API responde `{"status": "No puede mostrarse por restricciones de volumen"}`. Es
  `ErrTooLarge`; la misma tabla sin filtro o su CSV sí se sirven.
- **`TABLAS_OPERACION` está incompleta**: en noviembre de 2026, 57 de las 166 operaciones
  devolvían una lista vacía y, de ellas, 39 tenían tablas en la web que `DATOS_TABLA` sirve sin
  problema (sobre todo las "tpx"). `WebTables` las encuentra leyendo las páginas de INEbase:
  la pestaña de resultados de la operación (`&menu=resultados`) enlaza a índices
  `/dynt3/inebase/index.htm?padre=P&capsel=C`; cada índice despliega un capítulo con sus tablas
  (`/jaxi/Tabla.htm?tpx=N` o `/jaxiT3/Tabla.htm?t=N`) y enlaza los demás capítulos como
  `?padre=P&capsel=X`. `AllTables` usa la API y, si no lista nada, la web. Las 18 operaciones
  restantes solo publican xlsx, PDF, microdatos o un visor, o no tienen página en la API.

## Decisiones

- **Siempre `tip=AM`**: fechas como cadena, unidad y variables de cada serie. Sin él, las fechas
  llegan en milisegundos y la unidad es un id.
- **Tipos propios en inglés** (`Operation`, `Table`, `Series`, `Variable`, `Point`) con
  `UnmarshalJSON` que lee los nombres del INE. El JSON que escribe la CLI usa los nombres
  propios, nunca los del INE.
- Las tablas encontradas en la web llevan `FromWeb` y el capítulo en `Group`; la CLI las marca
  como `web` y la TUI como "vía ine.es". Se pide por id de operación, no por código, porque hay
  códigos repetidos (`OCE`, `CTNFSI`).
- `Point.Value` es un puntero: `nil` cuando el INE no publica el dato.
- `Ultima_Modificacion` se parsea en `Europe/Madrid` para que Go resuelva CET y CEST.
- **Descarga masiva** (`fetch`): workers en paralelo con un limitador común (200 ms por defecto
  entre peticiones, para no castigar al INE), escritura atómica (temporal y rename) para que
  una descarga cortada no se salte en la siguiente, y los ficheros existentes se saltan salvo
  `--force`. Cada ejecución deja un `manifest.json` con el resultado de cada fichero.
- **La librería (`ine`, `fetch`) no tiene dependencias externas** y no debe tenerlas: la CLI usa
  `flag` y solo `internal/tui` importa Bubble Tea v1, Bubbles y Lip Gloss.
- **TUI** (`internal/tui`, comando `ine tui`): operaciones → tablas → descarga. La selección
  sobrevive al cambiar de operación, así que se pueden juntar tablas de varias. Teclas en
  tablas: espacio marca, `a` marca las visibles, `f` cambia el formato, `n` la ventana de
  periodos, `d` descarga, `/` busca, `esc` vuelve. La búsqueda exige todas las palabras sin
  mirar tildes (`wordFilter`): el filtro difuso de Bubbles ordenaba mal frases largas.
- **Colores del INE**, sacados de www.ine.es: granate `#9E1834` (logo y titulares), verde
  azulado `#457E76` (barra de navegación), verde oscuro `#203C37` y menta `#DDEEEC`. Cada uno
  lleva un gemelo más claro para terminales oscuros (`styles.go`).
- El CSV del INE es siempre la tabla entera: `--last`, `--from` y `--to` solo afectan al JSON.

## Publicación

⚠️ El repo es **público** (MIT): ningún dato personal, nombre de persona ni de empresa en
ficheros, commits, tags ni releases.

- **Release**: `git tag vX.Y.Z && git push origin vX.Y.Z`. El workflow `release.yml` ejecuta
  GoReleaser (`.goreleaser.yaml`) y sube los binarios de macOS, Linux y Windows (amd64 y arm64)
  con `checksums.txt`. `main.version` se inyecta por `-ldflags`; con `go install` sale la
  versión del módulo.
- **Instalación**: `install.sh` baja `ine_<os>_<arch>.tar.gz` de la última release (los nombres
  no llevan versión a propósito, para que funcione `releases/latest/download`) y comprueba el
  checksum. Cambiar el `name_template` de los archivos rompe el script.
- `test.yml` pasa `make test` en cada push a `main` y en cada PR.

## Comandos

```bash
make test      # vet, gofmt y tests con -race
make build     # bin/ine
go run ./cmd/ine fetch --op IPC --last 12 --format json,csv -o ine-data
```

Los tests usan `httptest`; ninguno llama a la API real. La TUI se prueba a mano o dentro de
`tmux` (`send-keys` y `capture-pane`).

## Agent skills

### Triage labels

Las cinco etiquetas canónicas de triaje con su nombre por defecto: `needs-triage`, `needs-info`,
`ready-for-agent`, `ready-for-human` y `wontfix`. Ver `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` y `docs/adr/` en la raíz, creados cuando hagan falta. Ver
`docs/agents/domain.md`.
