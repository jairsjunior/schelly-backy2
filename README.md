# schelly-restic
This exposes the common functions of Backy2 with Schelly REST APIs so that it can be used as a backup backend for Schelly (backup scheduler), or standalone with curl, if you wish.

Backy2 is a great tool for creating Ceph image backups. For more, visit http://backy2.com/docs/

See more at http://github.com/flaviostutz/schelly

# Usage

docker-compose .yml

```
version: '3.5'

services:

  restic-api:
    image: flaviostutz/schelly-backy2
    ports:
      - 7070:7070
    environment:
      - LOG_LEVEL=debug
```

```
#create a new backup
curl -X POST localhost:7070/backups

#list existing backups
curl -X localhost:7070/backups

#get info about an specific backup
curl localhost:7070/backups/abc123

#remove existing backup
curl -X DELETE localhost:7070/backups/abc123

```

# REST Endpoints

As in https://github.com/flaviostutz/schelly#webhook-spec
