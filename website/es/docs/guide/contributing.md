# Cómo contribuir

Gately es un proyecto de código abierto impulsado por la comunidad. Las incidencias y los pull requests son bienvenidos. Repositorio: <https://github.com/wangwanjie/gately>

## Desarrollo local

Requisitos: Go 1.26+, Node.js 24+, pnpm.

```bash
make web      # compila el frontend (genera web/dist, integrado con go:embed)
make build    # compila bin/gately
make test     # go test -race ./...
make lint     # gofmt, go vet, comprobación de tipos del frontend
```

Para desarrollar el frontend, ejecuta `cd web && pnpm dev`; `/api` se redirige al `127.0.0.1:16680` local.

## Convenciones

- El código de compatibilidad debe basarse únicamente en el comportamiento, las interfaces y los formatos de configuración observables públicamente. No envíes código copiado de los binarios o del frontend de software de terceros.
- Un módulo nuevo necesita: una estructura de configuración con validación, arranque y parada en ejecución, endpoints REST y pruebas unitarias. Los módulos que admiten la importación de configuración implementan `luckyimport.Importer`, y los que deben mostrar su estado en la página de resumen implementan `module.Summarizer`.
- Los módulos opcionales se registran en `internal/modules/registry/mod_<módulo>.go`, con `//go:build !no_<módulo>` en el archivo, y se dan de alta también en `catalog.go` y en el `modules.json` de la raíz (`TestManifestMatchesCode` comprueba que los tres coincidan). La colaboración entre módulos (el WAF de los sitios, el despliegue de certificados por SSH y en Docker, etc.) se ofrece mediante funciones inyectadas que, si falta el otro módulo, responden «no incluido en esta compilación»; el código central no puede referenciar directamente los paquetes de los módulos opcionales. La CI ejecuta vet y la compilación con las combinaciones de tags de los tres preajustes.
- Los comentarios del código se escriben en chino; las sentencias de control deben usar llaves.

## Pruebas adicionales

Algunas pruebas se omiten por defecto y requieren una preparación adicional:

| Prueba | Cómo activarla |
| --- | --- |
| ACME de extremo a extremo (Pebble) | Instala pebble y pebble-challtestsrv, y define `GATELY_PEBBLE_BIN` y `GATELY_PEBBLE_SRC` |
| Servidores STUN públicos reales | `GATELY_NET_TEST=1` |
| Verificación de importación con una configuración real de Lucky | `GATELY_LUCKY_SAMPLE` y otras; consulta `TestImportRealSample` de cada módulo |

## Licencia

[Apache License 2.0](https://github.com/wangwanjie/gately/blob/main/LICENSE)

## Traducciones

- Los textos de la interfaz de administración están en `web/src/locales/<language>/<namespace>.json`. El idioma de origen es el chino simplificado; al añadir o cambiar textos, actualiza al menos `zh-CN` y `en`, y ejecuta `cd web && node scripts/check-i18n.mjs` antes de enviarlos. Consulta `web/src/locales/README.md` para ver las convenciones.
- Los paquetes de idioma del sitio web están en `website/.vitepress/i18n/<language>.json`, y las páginas de cada idioma están en `website/<language dir>/`.
- Los textos de ejecución del backend (mensajes de error, registros, notificaciones, documentación de la API) usan como clave las cadenas literales en chino del código fuente, con las traducciones en `internal/i18n/locales/<language>.json`. Tras añadir o cambiar texto en chino, ejecuta `go run ./internal/i18n/cmd/i18nsync -root .` para sincronizar las claves nuevas en todos los archivos de idioma y completa las traducciones; `go test ./internal/i18n` comprueba que todos los idiomas están completos y que los verbos de formato coinciden con el original. Para las frases que necesitan valores, usa una cadena de formato de `fmt.Sprintf` con la frase entera en lugar de unir varios fragmentos en chino, para que cada idioma pueda seguir su propio orden de palabras.
- La documentación de la API la genera el programa para cada idioma: `make api-docs` la escribe en `docs/api/<language>/`, así que no hay nada que sincronizar a mano.
- Idiomas disponibles: 简体中文, English, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Русский, Português (Brasil).
