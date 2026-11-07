# Ligne de commande et MCP

L'exécutable `gately` intègre deux sous-commandes : `gately ctl` appelle depuis le terminal l'API de gestion d'une instance en cours d'exécution, et `gately mcp` expose les mêmes API sous forme d'outils MCP pour les clients IA. Toutes deux accèdent à l'instance en HTTP avec des [jetons d'API](/fr/docs/reference/api) ; la liste des API provient du `/api/openapi.json` de l'instance et les permissions sont déterminées par le jeton.

Par ailleurs, `gately reset-password` et `gately reset-totp` agissent directement sur le répertoire de configuration local, sans passer par l'API, pour récupérer le compte administrateur après un mot de passe oublié ou un authentificateur perdu ; consultez [Réinitialiser le mot de passe et l'authentification à deux facteurs](/fr/docs/guide/install#reset).

`gately service install|uninstall|start|stop|restart|status` enregistre Gately comme service système démarré au boot ; voir [Démarrage automatique](/fr/docs/guide/install#service).

## Configuration de la connexion

Les informations de connexion sont déterminées dans l'ordre suivant, chaque étape remplaçant la précédente :

1. L'adresse par défaut `http://127.0.0.1:16680`
2. Le fichier de configuration `ctl.json`
3. Les variables d'environnement `GATELY_URL` et `GATELY_TOKEN`
4. Les options de ligne de commande `-url` et `-token`

Le fichier de configuration se trouve dans `gately/ctl.json` sous le dossier de configuration de l'utilisateur système (`~/.config/gately/ctl.json` sous Linux, `~/Library/Application Support/gately/ctl.json` sous macOS). Exemple de contenu :

```json
{
  "url": "http://127.0.0.1:16680",
  "token": "gtl_xxxx",
  "insecure": false
}
```

`gately ctl login` génère ce fichier.

## gately ctl

### Sous-commandes

| Commande | Description |
| --- | --- |
| `gately ctl ls [mot-clé]` | Liste les API accessibles au jeton, avec méthode, chemin et résumé ; filtrable par mot-clé |
| `gately ctl get <chemin>` | Envoie une requête GET |
| `gately ctl post <chemin> [JSON ou @fichier]` | Envoie une requête POST |
| `gately ctl put <chemin> [JSON ou @fichier]` | Envoie une requête PUT |
| `gately ctl delete <chemin> [JSON ou @fichier]` | Envoie une requête DELETE |
| `gately ctl login <adresse> <jeton>` | Vérifie la connexion et l'enregistre dans le fichier de configuration |

Le chemin doit commencer par `/api/`. Le corps de la requête peut être un littéral JSON, `@nomdefichier` (lecture d'un fichier) ou `-` (lecture de l'entrée standard), et doit être du JSON valide. La méthode `patch` est également disponible, et `ls` peut s'écrire `list`.

### Options de connexion

Les options se placent avant la sous-commande :

| Option | Description |
| --- | --- |
| `-url <adresse>` | Adresse de gestion, commençant par http ou https |
| `-token <jeton>` | Jeton d'API |
| `-insecure` | Ignore la vérification du certificat HTTPS, pour les instances de réseau local à certificat auto-signé |
| `-raw` | Affiche la réponse brute au lieu du JSON formaté |

En cas de succès, le code de sortie est 0 ; lorsque l'API renvoie un code autre que 2xx, la commande affiche `Erreur : HTTP code d'état : description` et se termine avec 1 ; un problème d'arguments se termine avec 2.

### Exemples

```bash
# Enregistrer la configuration de connexion (interroge d'abord /api/session pour vérifier le jeton)
gately ctl login http://192.168.1.2:16680 gtl_xxxx

# Lister les API accessibles au jeton, en filtrant les lignes contenant ddns
gately ctl ls ddns

# Lire l'état du système
gately ctl get /api/status

# Réveiller un appareil
gately ctl post /api/wol/devices/abc123/wake

# Désactiver une tâche DDNS
gately ctl put /api/ddns/tasks/abc/enable '{"enable":false}'

# Lire le corps de la requête depuis un fichier
gately ctl post /api/ddns/tasks @task.json

# Utiliser temporairement des variables d'environnement
GATELY_URL=https://gw.example.com GATELY_TOKEN=gtl_xxxx gately ctl get /api/status
```

::: tip Conseil
`ls` liste les API accessibles au jeton ; celles réservées aux sessions de connexion (compte, gestion des jetons, sauvegarde et restauration, etc.) n'apparaissent pas. L'autorisation des écritures dépend des permissions du jeton, et une permission insuffisante renvoie 403.
:::

## gately mcp

`gately mcp` fournit un service MCP sur l'entrée et la sortie standard (JSON-RPC, un message par ligne), lancé comme sous-processus par le client IA.

### Options

| Option | Description |
| --- | --- |
| `--url <adresse>` | Adresse de gestion |
| `--token <jeton>` | Jeton d'API |
| `--write` | Expose aussi les outils d'écriture |
| `--insecure` | Ignore la vérification du certificat HTTPS |

Les informations de connexion proviennent des mêmes sources que pour `gately ctl` (fichier de configuration, `GATELY_URL`, `GATELY_TOKEN`, options).

### Règles de génération des outils

- Après le démarrage du service, à la première demande de liste d'outils du client, le document OpenAPI de l'instance est lu et un outil est généré pour chaque API accessible au jeton.
- Le nom d'un outil combine méthode et chemin, comme `get_ddns_tasks` ou `put_ddns_tasks_key_enable`, avec 64 caractères au maximum ; un numéro est ajouté en cas de doublon.
- Les paramètres de chemin deviennent des paramètres de type chaîne obligatoires ; les outils GET ont un paramètre `query` facultatif (p. ex. `since=10`) ; les API avec corps de requête ont un paramètre `body` dont la structure provient d'OpenAPI.
- La description de l'outil contient le résumé de l'API, la méthode et le chemin, ainsi que la permission requise ; les outils portent l'annotation `readOnlyHint`, et les outils DELETE l'annotation `destructiveHint`.
- Un contenu renvoyé de plus de 200000 caractères est tronqué.

### Lecture seule par défaut

Sans `--write`, seuls les outils GET et HEAD sont exposés, et les API comme `.../secrets` qui renvoient des mots de passe en clair sont laissées de côté ; elles ne sont fournies qu'avec `--write`. Ajoutez `--write` pour que l'IA puisse modifier la configuration. Même avec l'écriture activée, les permissions du jeton restent en vigueur : le plus prudent est donc de créer un jeton dédié à MCP, avec uniquement les permissions nécessaires.

::: warning Attention
`--write` autorise le client IA à appeler toutes les API d'écriture permises par le jeton, suppression comprise. Validez d'abord le déroulement avec un jeton en lecture seule, puis accordez l'écriture selon les besoins.
:::

### Configuration du client

Pour Claude Desktop, Cursor et les autres clients qui utilisent le format de configuration `mcpServers` :

```json
{
  "mcpServers": {
    "gately": {
      "command": "gately",
      "args": ["mcp"],
      "env": {
        "GATELY_URL": "http://127.0.0.1:16680",
        "GATELY_TOKEN": "gtl_xxxx"
      }
    }
  }
}
```

Pour activer l'écriture, remplacez `args` par `["mcp", "--write"]`. `command` peut contenir le chemin absolu de `gately`.

Claude Code s'ajoute en ligne de commande :

```bash
claude mcp add gately -e GATELY_URL=http://127.0.0.1:16680 -e GATELY_TOKEN=gtl_xxxx -- gately mcp
```
