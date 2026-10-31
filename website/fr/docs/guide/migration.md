# Migrer de Lucky vers Gately

> Gately est un projet indépendant, sans affiliation avec Lucky. La compatibilité repose sur des formats observables publiquement.

Gately lit directement les fichiers de configuration `.lkcf` de Lucky, ou le zip du site entier exporté par la « sauvegarde de configuration » de Lucky, et migre les règles, les tâches et les certificats. Vous n'avez pas besoin de désinstaller Lucky : les deux peuvent cohabiter et vous basculez module par module.

::: tip Migrer depuis nginx
Ce guide porte sur la migration depuis Lucky. Les sites nginx existants s'importent directement via « Services Web → Importer nginx » : voir [Services Web · Importer une configuration nginx](/fr/docs/modules/web#import-nginx).
:::

## 1. Préparation

1. Repérez le dossier de configuration de Lucky : pour un déploiement Docker, c'est généralement le dossier de l'hôte mappé sur `/app/conf` dans le conteneur, qui contient `lucky_base.lkcf`, `lucky_ddns.lkcf`, etc.
2. Déployez Gately (voir le README). Recommandations :
   - utilisez `--network host` pour écouter directement sur les ports de l'hôte ;
   - le port d'administration par défaut est 16680, sans conflit avec le 16601 de Lucky ;
   - si les scripts ou les exports de certificats de Lucky utilisent des chemins internes au conteneur (par exemple `/etc/lucky/custom_scripts`, `/data/nginx/...`), montez-les dans le conteneur Gately sur le **même chemin** : aucune modification ne sera nécessaire après l'import.

## 2. Import

Ouvrez « Paramètres → Sauvegarde et migration » dans Gately, sélectionnez tous les fichiers `lucky_*.lkcf`, ou directement le zip de configuration du site entier exporté par Lucky (les `.lkcf` de l'archive sont reconnus automatiquement, les autres fichiers ignorés). Cliquez d'abord sur « Aperçu » pour voir combien d'éléments chaque module peut importer et quels avertissements apparaissent, puis, après vérification, cliquez sur « Confirmer l'import ».

Importer plusieurs fois est sans risque : les règles, tâches et certificats portant le même nom qu'un élément existant sont ignorés.

Fichiers pris en charge :

| Fichier | Importé dans | Remarques |
| --- | --- | --- |
| lucky_portforward.lkcf | Redirection de ports | TLS/chiffrement de flux et limitation de débit ne sont pas encore pris en charge, un avertissement s'affiche |
| lucky_ddns.lkcf | DNS dynamique | 11 fournisseurs pris en charge ; les adresses officielles de recherche d'IP de Lucky sont retirées |
| lucky_stun.lkcf | Traversée STUN | Inclut la liste globale des serveurs STUN et le webhook global |
| lucky_ssl.lkcf | Certificats SSL | Les certificats déjà émis sont importés tels quels, sans nouvelle demande avant leur expiration |
| lucky_reverseproxy.lkcf | Services Web | Sites, authentification, références WAF (si le WAF d'une sous-règle vaut main, l'instance de la règle d'écoute est reprise) ; les variables des sous-règles de redirection comme `{host}{path}{args}`, `{port}`, `{1}` et `{STUN_NomRègle_PORT}` continuent de fonctionner |
| lucky_coraza.lkcf | Pare-feu WAF | Reprend les clés d'instance, les références des sites restent valides |
| lucky_wol.lkcf | Wake-on-LAN | |
| lucky_webterminal.lkcf | Terminal Web | Connexions SSH et Telnet, proxys, `keyFile`, `workDir`, délais, variables d'environnement, dossiers d'accès rapide, paramètres de session, raccourcis globaux, ainsi que le contrat d'utilisation et le secret 2FA ; pour les connexions SSH, l'empreinte de l'hôte est enregistrée à la première connexion |
| lucky_webdav.lkcf | WebDAV | La liste de montages d'un utilisateur devient plusieurs dossiers, les utilisateurs de même nom et même mot de passe sont fusionnés ; les montages référençant la gestion du stockage sont résolus vers le dossier local correspondant |
| lucky_ftpserver.lkcf | FTP | Port d'écoute, plage de ports passifs, TLS et délais, ainsi que les utilisateurs (traités comme pour WebDAV) |
| lucky_storagemanagement.lkcf | Montages de stockage | Dossiers locaux et stockages WebDAV ; Aliyun Drive, montages système et proxy d'accès ne sont pas migrés |
| lucky_frp.lkcf | frp | Première instance cliente (adresse du serveur, jeton, protocole de transport et proxys) et première instance serveur ; visiteurs, proxys de plugin et tableau de bord non migrés |
| lucky_cloudflared.lkcf | Tunnel Cloudflare | Tunnels utilisant un jeton de tunnel (avec leur jeton d'API) ; les clients Access et les tunnels renseignés uniquement avec un jeton d'API ne sont pas migrés |
| lucky_base.lkcf | Paramètres globaux, Services Web | Accès limité au réseau local, nombre maximal d'échecs de connexion, durée de session, fuseau horaire, DNS personnalisé (y compris la notation doh@), paramètres GC, sources de confiance du protocole PROXY. Port d'administration, chemin d'accès secret, compte administrateur et authentification à deux facteurs ne sont pas migrés |

Les tâches planifiées, le filtrage d'IP et les réglages Docker de Lucky sont vides dans les configurations courantes et ne sont pas migrés pour l'instant ; configurez-les manuellement s'ils contiennent quelque chose.

**Les règles importées sont désactivées par défaut** : cela évite, après un import sur une machine de test, de faire pointer des domaines de production vers de mauvaises adresses, ou d'entrer en concurrence avec un Lucky encore actif pour les ports et les mappages UPnP du routeur. Lorsque Gately tourne bien sur l'appareil cible et que Lucky est arrêté, cochez « Conserver l'état d'activation de Lucky après l'import » : les règles fonctionnent alors immédiatement selon leur état dans Lucky (y compris l'interrupteur général de chaque module).

## 3. Bascule

Basculez module par module et vérifiez chaque étape :

1. **Certificats** : activez le certificat dans Gately. À l'activation, il écrit le certificat dans le dossier de destination et exécute le script de changement (mêmes noms de fichiers que Lucky) ; vérifiez que le dossier est accessible en écriture.
2. **DDNS** : désactivez la tâche dans Lucky → activez la tâche correspondante dans Gately → cliquez sur « Forcer la synchronisation » pour confirmer que l'enregistrement est à jour.
3. **Redirection de ports / STUN** : désactivez d'abord la règle dans Lucky (pour libérer le port et le mappage UPnP), puis activez-la dans Gately. Après l'activation d'une règle STUN, le port public peut changer ; les scripts et les webhooks vous en informent automatiquement.
4. **Services Web** : désactivez la règle d'écoute de Lucky → activez-la dans Gately. Un site HTTPS exige un certificat activé du module de certificats couvrant le domaine.
5. Arrêtez le conteneur Lucky seulement quand tout est confirmé.

## 4. Montages pour un déploiement Docker

Les scripts et les exports de certificats de Lucky utilisent des chemins internes au conteneur Lucky ; le conteneur Gately doit monter les mêmes dossiers de l'hôte sur les mêmes chemins. Reprenez le compose de Lucky ligne par ligne, par exemple :

```yaml
volumes:
  - ./config:/etc/gately
  - /var/run/docker.sock:/var/run/docker.sock
  # Si Lucky exporte les certificats vers /data/nginx/ssl, montez le même dossier de l'hôte sur le même chemin
  - /srv/nginx/ssl:/data/nginx/ssl
  # Scripts appelés par les règles (vérifiez que le chemin saisi dans Lucky existe réellement dans le conteneur Lucky)
  - /srv/lucky/custom_scripts:/etc/lucky/custom_scripts:ro
```

L'image Gately embarque `sh`, `bash` et `curl` : les scripts de notification courants s'exécutent directement.

## 5. Éléments à traiter manuellement

- **Connexion via des tiers** : les connexions GitHub/QQ/Google de Lucky passent par le serveur de relais officiel de Lucky ; Gately utilise vos propres applications OAuth. Lors de l'import, sélectionnez aussi `lucky_thirdPartyAuthManager.lkcf` (table des identités), `lucky_reverseproxy.lkcf` et `lucky_base.lkcf` :
  - Les identités tierces référencées par les sites et la console d'administration sont migrées en liste d'autorisation d'après la table des identités (ID numérique pour GitHub, sub pour Google, openid pour QQ ; nom d'utilisateur et e-mail sont inscrits en remarque).
  - Le mode de connexion de la console se règle dans « Paramètres → Compte et sécurité → Connexion via des tiers », celui des sites dans « Services Web → Paramètres globaux » ; il ne prend effet qu'après saisie de votre Client ID/Secret et de l'URL de rappel, puis activation, l'import ne l'active pas. Les identifiants de fournisseur à utiliser sont `github`, `google` et `qq`, afin de correspondre aux identités migrées.
  - L'openid de QQ diffère selon l'application QQ Connect : avec votre propre application, la première connexion est refusée et un nouvel identifiant de compte s'affiche ; ajoutez-le à la liste d'autorisation.
  - Les sites qui ne reposent que sur la connexion via des tiers et dont aucune identité migrable n'a été trouvée restent désactivés après l'import.
- **Certificats ZeroSSL** : les identifiants EAB de Lucky sont réutilisés pour enregistrer un nouveau compte ACME ; si ZeroSSL refuse, régénérez l'EAB dans la console ZeroSSL.
- **Instances frp** : Gately n'a qu'un frpc et un frps ; les instances excédentaires de Lucky sont listées dans le rapport, déplacez-les vers d'autres machines ou fusionnez-les dans la même instance. Un frpc de Gately déjà configuré vers un autre serveur ou un frps déjà activé n'est pas écrasé. Le compte du tableau de bord frps est géré par Lucky : après la migration, redéfinissez le port et le mot de passe si vous voulez un tableau de bord.
- **Références de stockage** : les utilisateurs FTP / WebDAV de Lucky peuvent monter les stockages de la « gestion du stockage ». Lors de l'import, sélectionnez aussi `lucky_storagemanagement.lkcf` (ou importez directement le zip du site entier) ; les stockages locaux sont résolus vers les dossiers correspondants. Les stockages WebDAV, Aliyun Drive et les montages rclone ne sont pas encore pris en charge et sont signalés dans le rapport.
- **Fonctions de Lucky pas encore implémentées** : DLNA, montage de disques cloud rclone, Aliyun Drive, base d'IP (`lucky_ipdb.lkcf`), etc. ne sont pas migrés et sont ignorés à l'import.
- **Serveurs officiels de Lucky** : Gately ne se connecte jamais à des adresses comme `66666.host` ou `lucky666.cn` ; après la migration, la recherche de l'IP publique utilise des services publics tiers (modifiables dans les tâches DDNS).

## 6. Retour arrière

Gately ne modifie aucun fichier de Lucky. Pour revenir en arrière, désactivez les règles correspondantes dans Gately, puis réactivez-les dans Lucky.
