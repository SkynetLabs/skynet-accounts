# skynet-accounts

`skynet-accounts` is a service implementing user accounts for [Skynet](https://siasky.net) portals. It uses MongoDB for data storage.

## Setup steps

### The `.env` file

All local secrets are loaded from a `.env` file in the root directory of the project.

Those are (example values):
```.env
SKYNET_DB_HOST="localhost"
SKYNET_DB_PORT="27017"
SKYNET_DB_USER="username"
SKYNET_DB_PASS="password"
SKYNET_DB_PEPPER="strong random string acting as a secret salt for user passwords"
JWT_SECRET="strong secret key for signing JWT tokens"
COOKIE_DOMAIN="siasky.net"
COOKIE_HASH_KEY="strong random hashing key, at least 32 bytes long"
COOKIE_ENC_KEY="strong random encryption key, 16 or 32 bytes long. Only needed if you want encrypted cookies. Recommended!"
```

## Recommended reading
- [JSON and BSON](https://www.mongodb.com/json-and-bson)
- [Using the official MongoDB Go driver](https://vkt.sh/go-mongodb-driver-cookbook/)

