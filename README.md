<p align="center">
  <img src="docs/banner.jpg" alt="Fachada modernista de tejado ondulado, balcones curvos y dos torres, en granate y verde azulado sobre menta" width="100%">
</p>

# ine

Cliente en Go y CLI para navegar el catálogo del INE y descargar de golpe los datos de muchas
de sus tablas, en JSON normalizado o en el CSV que publica el propio INE.

| | |
|---|---|
| **Qué es** | Librería y CLI en Go sobre la API JSON del INE (Tempus3) |
| **Estado** | En desarrollo: librería, CLI y TUI hechas |
| **Stack** | Go 1.26; la TUI con Bubble Tea, Bubbles y Lip Gloss |
| **Repo** | `gverdugo-dev/ine`, público, licencia MIT |

## Qué hace

| Comando | Qué hace |
|---------|----------|
| `ine ops [filtro]` | Lista las operaciones estadísticas (IPC, EPA, ...) |
| `ine tables <operación> [filtro]` | Lista las tablas de una operación. Si la API no lista ninguna, las busca en las páginas de INEbase (`--web` lo fuerza) |
| `ine get <tabla>` | Muestra las series de una tabla (por defecto, el último periodo) |
| `ine series <código>` | Muestra una serie |
| `ine fetch [tablas...] [-f lista.txt] [--op IPC]` | Descarga muchas tablas a disco |
| `ine tui [-o dir]` | Navega operaciones y tablas, marca las que quieras y las descarga, con los colores del INE |

Todos aceptan `--json` y `--lang ES|EN`; `get`, `series` y `fetch` aceptan `--last N`,
`--from` y `--to`. Los filtros ignoran mayúsculas y tildes.

La lista de tablas por operación de la API del INE está incompleta: muchas operaciones (I+D,
TIC, cuentas medioambientales, salud, discapacidad...) salen vacías aunque sus tablas se puedan
descargar. `ine` las encuentra leyendo la web de INEbase y las descarga por la API como las
demás. Lo que el INE solo publica en xlsx, PDF o visores queda fuera.

`fetch` descarga en paralelo (`-j`, 4 por defecto) con un mínimo de 200 ms entre peticiones
(`--interval`), escribe `<tabla>.json` y/o `<tabla>.csv` (`--format json,csv`), salta lo que ya
existe salvo `--force` y deja un `manifest.json` con el resultado de cada fichero. Sale con
código 1 si alguna tabla falla.

## Instalación

macOS y Linux, con el binario de la última release (comprueba su checksum):

```bash
curl -fsSL https://raw.githubusercontent.com/gverdugo-dev/ine/main/install.sh | sh
```

Lo deja en `/usr/local/bin` si se puede escribir ahí y si no en `~/.local/bin`
(`INE_INSTALL_DIR` lo cambia; `INE_VERSION=v0.1.0` fija una versión). Con Go instalado:

```bash
go install github.com/gverdugo-dev/ine/cmd/ine@latest
```

En Windows, el zip de [Releases](https://github.com/gverdugo-dev/ine/releases).

## Cómo se usa

```bash
ine tui                              # la interfaz interactiva
ine ops precios
ine tables IPC ecoicop
ine get 50902 --last 3
ine fetch --op IPC --last 24 --format json,csv -o datos/ipc
ine fetch -f informes.txt -o datos   # un id por línea, # para comentarios
ine tui -o datos                     # espacio marca, f formato, n periodos, d descarga
```

Desde el código: `make install` (o `make build`, que deja `bin/ine`).

Como librería:

```go
c := ine.New()
series, err := c.TableData(ctx, "50902", ine.Query{Last: 12})
```

## Estructura

```
.
├── client.go    # cliente Tempus3: peticiones, reintentos, errores
├── types.go     # Operation, Table, Series, Variable, Point
├── fetch/       # descarga masiva en paralelo
├── cmd/ine/     # la CLI
├── internal/tui # la TUI (Bubble Tea)
└── docs/agents/ # configuración de las skills de ingeniería
```

## Cómo encaja

No tiene servicio ni base de datos: solo lee la API pública del INE y deja ficheros en disco,
listos para cualquier herramienta de análisis.

## Más información

Las decisiones y los detalles de la API están en `CLAUDE.md`.
