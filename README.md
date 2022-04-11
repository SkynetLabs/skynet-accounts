# skynet-accounts

`skynet-accounts` is a service that stores [Skynet](https://siasky.net) user account data. It uses MongoDB for data
storage.

## Setup steps

### The `.env` file

All local secrets are loaded from a `.env` file in the root directory of the project.

Those are (example values):

```.env
ACCOUNTS_EMAIL_URI="smtps://<email address>:<email password>@<smtp server for email>/?skip_ssl_verify=true"
ACCOUNTS_JWKS_FILE="/accounts/conf/jwks.json"
COOKIE_DOMAIN="siasky.net"
COOKIE_HASH_KEY="any thirty-two byte string is ok"
COOKIE_ENC_KEY="any thirty-two byte string is ok"
PORTAL_DOMAIN="https://siasky.net"
SERVER_DOMAIN="eu-ger-1.siasky.net"
SKYNET_DB_HOST="localhost"
SKYNET_DB_PORT="27017"
SKYNET_DB_USER="username"
SKYNET_DB_PASS="password"
STRIPE_API_KEY="put-your-key-here"
STRIPE_WEBHOOK_SECRET="put-your-secret-here"
```

There are some optional ones, as well:

```.env
ACCOUNTS_EMAIL_FROM="norepl@siasky.net"
SKYNET_ACCOUNTS_LOG_LEVEL=trace
ACCOUNTS_MAX_NUM_API_KEYS_PER_USER=1000
```

Meaning of environment variables:

* ACCOUNTS_EMAIL_URI is the full email URI (including credentials) for sending emails.
  example `ACCOUNTS_EMAIL_URI=smtps://hello@gmail.com:MYSUP3R$TRONGPW@smtp.gmail.com:465/?skip_ssl_verify=false`
* ACCOUNTS_EMAIL_FROM allows us to set the FROM email on our outgoing emails. If it's not set we will use the user from
  ACCOUNTS_EMAIL_URI.
* ACCOUNTS_JWKS_FILE is the file which contains the JWKS `accounts` uses to sign the JWTs it issues for its users. It
  defaults to `/accounts/conf/jwks.json`. This file is required.
* COOKIE_DOMAIN defines the domain for which we set the login cookies. It usually matches PORTAL_DOMAIN.
* COOKIE_HASH_KEY and COOKIE_ENC_KEY are used for securing the cookie which holds the user's JWT token.
* PORTAL_DOMAIN is the domain for which we issue our JWTs.
* SERVER_DOMAIN defines the domain name of the current server in a cluster setup. In a single server setup it should
  match PORTAL_DOMAIN.
* SKYNET_ACCOUNTS_LOG_LEVEL defines the log level used by the service. It goes from `trace` to `panic`. The recommended
  value is `info`.
* SKYNET_DB_HOST, SKYNET_DB_PORT, SKYNET_DB_USER, and SKYNET_DB_PASS tell `accounts` how to connect to the MongoDB
  instance it's supposed to use.
* STRIPE_API_KEY, STRIPE_WEBHOOK_SECRET allow us to process user payments made via Stripe.
* ACCOUNTS_MAX_NUM_API_KEYS_PER_USER defines the maximum number of API keys a user can create. If a user needs to add a
  new key after reaching that number, they would need to first delete another.

### Generating a JWKS and Cookie Keys

The JSON Web Key Set is a set of cryptographic keys used to sign the JSON Web Tokens `accounts` issues for its users.
These tokens are used to authorize users in front of the service and are required for its operation.

You can generate the necessary `jwks.json` file and proper keys for the two `COOKIE` variables by cloning this repo and
running:

```
git clone https://github.com/SkynetLabs/skynet-accounts.git
cd skynet-accounts
make docker-generate
```

This will generate the needed information in an `output/` directory. The `COOKIE`
variables are in the `output/env` file and the JWKS is in the `output/jwks.json`
file.

## License

Skynet Accounts uses a custom [License](./LICENSE.md). The Skynet License is a source code license that allows you to
use, modify and distribute the software, but you must preserve the payment mechanism in the software.

For the purposes of complying with our code license, you can use the following Siacoin address:

`fb6c9320bc7e01fbb9cd8d8c3caaa371386928793c736837832e634aaaa484650a3177d6714a`

## Recommended reading

- [JSON and BSON](https://www.mongodb.com/json-and-bson)
- [Using the official MongoDB Go driver](https://vkt.sh/go-mongodb-driver-cookbook/)
