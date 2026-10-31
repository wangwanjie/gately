# Certificats SSL

Émettez et renouvelez automatiquement des certificats pour les [Services Web](/fr/docs/modules/web), ou exportez-les vers d'autres logiciels.

Lorsque plusieurs certificats couvrent le même domaine, celui qui expire le plus tard est utilisé ; l'ordre et les groupes de la liste des certificats n'affectent que l'affichage.

## Fonctions

- Émission et renouvellement automatiques **ACME DNS-01**, en réutilisant les fournisseurs DNS du module [DNS dynamique](/fr/docs/modules/ddns), sans avoir à ouvrir les ports 80 / 443 ;
- **ACME HTTP-01** : aucun fournisseur DNS requis, mais le port 80 de chaque domaine doit être joignable depuis Internet ; les certificats génériques ne sont pas pris en charge ;
- Certificats génériques (joker) et EAB (par exemple ZeroSSL) ;
- **Certificats de courte durée et certificats d'adresse IP** : avec le profil de courte durée de Let's Encrypt (validité d'environ 6 jours), des certificats peuvent être émis pour des adresses IP publiques ;
- Import d'un certificat existant, ou lecture depuis des fichiers ;
- Mappage vers un répertoire : après un changement de certificat, les fichiers sont écrits dans le répertoire indiqué et un script est exécuté, afin que les autres services puissent se recharger ;
- Déploiement automatique : après un changement de certificat, il est copié vers plusieurs répertoires locaux, envoyé par SSH vers d'autres hôtes avec exécution d'une commande, ou les conteneurs Docker qui l'utilisent sont redémarrés.

## Validation DNS-01

Choisissez DNS-01 dans la « Méthode » du certificat, puis sélectionnez un fournisseur DNS. Lors de l'émission, Gately ajoute l'enregistrement TXT `_acme-challenge` via l'API du fournisseur et le supprime une fois la validation terminée.

Le « Domaine racine » est la zone DNS qui contient l'enregistrement. S'il est laissé vide, Gately recherche la zone faisant autorité pour l'enregistrement de validation au moyen d'une requête SOA : un sous-domaine délégué à un autre fournisseur DNS (par exemple `web.example.com` hébergé séparément de `example.com`) et les suffixes à plusieurs niveaux comme `example.com.cn` sont ainsi correctement détectés. Si la recherche échoue, la zone est déduite du nom de domaine et la raison est consignée dans le journal. Vous pouvez aussi saisir vous-même le domaine racine, par exemple `web.example.com`.

## Réseau d'émission

L'option « Réseau » du certificat limite le réseau sortant utilisé lors de l'émission : « Automatique » laisse le système choisir, et « IPv4 uniquement » ou « IPv6 uniquement » fait que l'accès à l'autorité de certification, les appels à l'API du fournisseur DNS et la vérification de la propagation de l'enregistrement TXT n'utilisent que cette famille d'adresses. Si l'hôte a une adresse IPv6 mais qu'IPv6 ne fonctionne pas réellement, la vérification de propagation en mode automatique peut expirer lors de l'interrogation des serveurs DNS faisant autorité ; choisissez alors « IPv4 uniquement ».

## Validation HTTP-01

Choisissez HTTP-01 dans la « Méthode » du certificat. Lors de l'émission, l'AC accède à `http://<domaine>/.well-known/acme-challenge/<token>` :

- Si un [service Web](/fr/docs/modules/web) écoute déjà sur le port 80, c'est lui qui répond, sans incidence sur les sites existants ;
- Si le port 80 est libre, Gately l'écoute temporairement pendant l'émission et le ferme aussitôt après ;
- Si le routeur redirige le port public 80 vers un autre port de cette machine, indiquez ce port dans « Port temporaire ».

De nombreux accès Internet résidentiels (en Chine continentale notamment) bloquent le port 80 entrant : utilisez alors DNS-01.

## Certificats de courte durée et certificats d'adresse IP

Le « Profil de certificat » correspond au profile ACME :

- **Par défaut** : choisi par l'AC, 90 jours chez Let's Encrypt ;
- **Courte durée (certificat IP)** : le profil `shortlived` de Let's Encrypt, valable environ 6 jours. Gately le renouvelle automatiquement avant l'expiration selon « Heures d'avance pour les certificats de courte durée » dans « Paramètres de renouvellement » (24 heures par défaut) et réessaie toutes les heures en cas d'échec ;
- Vous pouvez aussi saisir un autre nom de profil pris en charge par l'AC.

Saisissez une adresse IP publique (IPv4 ou IPv6, par exemple `203.0.113.10` ou `2001:db8::1`) dans les domaines pour demander un certificat d'adresse IP ; un même certificat peut contenir des IP et des domaines. Conditions :

- Utiliser Let's Encrypt, choisir le profil de courte durée et HTTP-01 dans la « Méthode ». L'AC accède à `http://<IP>/.well-known/acme-challenge/<token>`, le port 80 de cette IP doit donc être joignable depuis Internet. Une fois l'IP saisie, l'éditeur affiche une indication : il suffit de cliquer sur « Configurer automatiquement » ;
- Les adresses privées, les adresses de bouclage et `100.64.0.0/10` ne peuvent pas passer la validation d'une AC publique ; une AC personnalisée (par exemple un serveur ACME auto-hébergé sur le réseau local) n'a pas ces restrictions ;
- ZeroSSL et Google Trust Services n'émettent pas de certificats d'adresse IP via ACME.

Lors d'un accès HTTPS par adresse IP, le navigateur n'envoie pas de nom de domaine (SNI) : Gately choisit alors le certificat IP correspondant à l'adresse locale de la connexion ; derrière un NAT ou une redirection de port, lorsque l'adresse locale diffère de l'IP publique du certificat, le certificat IP qui expire le plus tard est utilisé.

Quand le profil d'un certificat déjà émis passe de par défaut à courte durée ou inversement, le certificat est réémis immédiatement avec le nouveau profil. Pour les certificats de courte durée, le rappel d'expiration n'est envoyé que lorsque le temps restant passe sous la moitié des heures d'avance, signe que le renouvellement a échoué plusieurs fois.

## Mappage de certificat

Une fois un certificat activé, Gately écrit `.crt`, `.key`, `.pem` et `_issuerCertificate.crt` dans le répertoire de mappage et exécute le script de changement.

## Déploiement automatique

Dans « Déploiement automatique » d'un certificat, vous pouvez ajouter plusieurs cibles de déploiement, exécutées automatiquement après l'émission, le renouvellement ou la mise à jour du certificat :

- **Copier vers un répertoire local** : écrit le certificat dans un répertoire de cette machine, puis exécute localement la commande « Exécuter après la copie ». Vous pouvez ajouter plusieurs répertoires, chacun avec une commande différente. Si les fichiers d'un répertoire sont supprimés ou modifiés, la vérification suivante les recopie. Si Gately tourne dans Docker, le répertoire et la commande sont dans le conteneur ;
- **Envoyer vers un hôte SSH** : choisissez une connexion SSH existante du [terminal web](/fr/docs/modules/terminal), dont l'adresse, la méthode d'authentification et le proxy sont réutilisés. Les fichiers du certificat (`<note>.crt`, `.key`, `.pem`, `_issuerCertificate.crt`, comme pour le mappage de certificat : la note est donc obligatoire) sont écrits dans le « Répertoire distant », puis la commande « Exécuter après l'envoi » s'exécute sur l'hôte distant, par exemple `systemctl reload nginx` ; elle peut utiliser `$certName`, `$certDir`, `$certFile` et `$keyFile`. Chaque fichier est d'abord écrit dans un fichier temporaire avec ses permissions, puis renommé pour remplacer l'ancien ; la clé privée a les permissions 0600 ;
- **Redémarrer des conteneurs locaux** : après un changement de certificat, les conteneurs Docker choisis sont redémarrés l'un après l'autre. Utile pour les services dont le conteneur monte le répertoire de mappage et doit redémarrer pour charger le nouveau certificat.

« Copier vers un répertoire local » et « Envoyer vers un hôte SSH » acceptent des noms de fichiers pour le certificat (chaîne complète) et la clé privée, comme `fullchain.pem` et `privkey.pem`. Si les deux sont renseignés, seuls ces deux fichiers sont écrits et la note peut rester vide ; laissés vides, les noms du mappage des certificats sont utilisés. `$certFile` et `$keyFile` désignent les fichiers réellement écrits.

Le déploiement automatique ne fait jamais confiance à un hôte inconnu : connectez-vous une fois à la connexion SSH choisie depuis le terminal web pour enregistrer sa clé d'hôte avant tout envoi. En cas d'échec, le déploiement est retenté toutes les heures et déclenche l'événement « Certificat : échec du déploiement », auquel vous pouvez vous abonner dans le [Centre de notifications](/fr/docs/modules/notify). Le bouton « Déployer maintenant » de la carte du certificat ignore l'intervalle de nouvel essai, déploie immédiatement vers toutes les cibles activées et affiche le résultat de chacune.

Lors de la modification d'un certificat, les déploiements dont les réglages de cible n'ont pas changé ne sont pas réexécutés ; modifier le répertoire distant, la commande ou les conteneurs déclenche un nouveau déploiement.

## Note de sécurité

Les clés privées sont stockées dans `ssl.json`, dans le répertoire de configuration. Conservez ce fichier en lieu sûr, et assurez-vous que le dépôt est privé si vous le placez sous gestion de versions.
