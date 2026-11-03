# Contribuer

Gately est un projet open source porté par la communauté. Les tickets et les pull requests sont les bienvenus. Dépôt : <https://github.com/wangwanjie/gately>

## Développement local

Prérequis : Go 1.26+, Node.js 24+, pnpm.

```bash
make web      # compile le frontend (sortie dans web/dist, intégré via go:embed)
make build    # compile bin/gately
make test     # go test -race ./...
make lint     # gofmt, go vet, vérification des types du frontend
```

Pour le développement du frontend, lancez `cd web && pnpm dev` ; `/api` est relayé vers `127.0.0.1:16680` en local.

## Conventions

- Le code de compatibilité doit reposer uniquement sur des comportements, interfaces et formats de configuration observables publiquement. Ne soumettez pas de code copié des binaires ou du frontend d'un logiciel tiers.
- Un nouveau module requiert : une structure de configuration avec validation, un démarrage/arrêt à l'exécution, des points d'accès REST et des tests unitaires. Les modules qui prennent en charge l'import de configuration implémentent `luckyimport.Importer`, et ceux qui doivent afficher leur état sur la page de vue d'ensemble implémentent `module.Summarizer`.
- Un module optionnel s'enregistre dans `internal/modules/registry/mod_<module>.go`, fichier marqué `//go:build !no_<module>`, et doit aussi figurer dans `catalog.go` et dans `modules.json` à la racine (`TestManifestMatchesCode` vérifie la cohérence des trois). La collaboration entre modules (WAF des sites, déploiement de certificats vers SSH et Docker, etc.) passe par des fonctions injectées qui renvoient « non inclus dans ce build » quand l'autre module manque ; le code central ne doit pas référencer directement les paquets des modules optionnels. La CI exécute vet et la compilation avec les combinaisons de tags des trois préréglages.
- Les commentaires du code sont rédigés en chinois ; les instructions de contrôle doivent utiliser des accolades.

## Tests supplémentaires

Certains tests sont ignorés par défaut et demandent une préparation supplémentaire :

| Test | Comment l'activer |
| --- | --- |
| ACME de bout en bout (Pebble) | Installez pebble et pebble-challtestsrv, puis définissez `GATELY_PEBBLE_BIN` et `GATELY_PEBBLE_SRC` |
| Vrais serveurs STUN publics | `GATELY_NET_TEST=1` |
| Vérification de l'import avec une vraie configuration Lucky | `GATELY_LUCKY_SAMPLE` et autres ; voir `TestImportRealSample` de chaque module |

## Licence

[Apache License 2.0](https://github.com/wangwanjie/gately/blob/main/LICENSE)

## Traductions

- Les textes de l'interface d'administration se trouvent dans `web/src/locales/<language>/<namespace>.json`. La langue source est le chinois simplifié ; quand vous ajoutez ou modifiez des textes, mettez à jour au moins `zh-CN` et `en`, et lancez `cd web && node scripts/check-i18n.mjs` avant de soumettre. Voir `web/src/locales/README.md` pour les conventions.
- Les packs de langue du site se trouvent dans `website/.vitepress/i18n/<language>.json`, et les pages de chaque langue dans `website/<language dir>/`.
- Les textes d'exécution du backend (messages d'erreur, journaux, notifications, documentation de l'API) ont pour clés les chaînes littérales chinoises du code source, avec les traductions dans `internal/i18n/locales/<language>.json`. Après avoir ajouté ou modifié du texte chinois, lancez `go run ./internal/i18n/cmd/i18nsync -root .` pour synchroniser les nouvelles clés dans chaque fichier de langue et compléter les traductions ; `go test ./internal/i18n` vérifie que chaque langue est complète et que les verbes de format correspondent à la source. Pour les phrases qui doivent intégrer des valeurs, utilisez une chaîne de format `fmt.Sprintf` couvrant la phrase entière plutôt que d'assembler plusieurs fragments chinois, afin que chaque langue puisse suivre son propre ordre des mots.
- La documentation de l'API est générée par langue par le programme : `make api-docs` l'écrit dans `docs/api/<language>/` ; il n'y a donc rien à synchroniser à la main.
- Langues prises en charge : 简体中文, English, 繁體中文, 日本語, 한국어, Español, Français, Deutsch, Русский, Português (Brasil).
