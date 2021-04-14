# API Guide

## General terms

### ORY, Kratos, Oathkeeper, and JWx

While `skynet-accounts` handles account information in the context of a Skynet portal, the baseline account management (
account CRUD, email verification, password resets, etc.) is handled by [ORY](https://www.ory.sh/)
([Kratos](https://www.ory.sh/kratos/) and [Oathkeeper](https://www.ory.sh/oathkeeper/))
to which we often refer to as "Kratos". This also covers the login/logout process and the issuance of JTW tokens. When
we talk about JWT (or JWK, or JWKS)
we mean the tokens issued by ORY.

The workflow of verification follows a simple pattern:

* Oathkeeper exposes a public link on which it shares the public keys with which anyone can verify the validity of the
  JWT tokens it issues.
* `skynet-accounts` fetches those keys and uses them to validate the JWTs it receives in requests.

### User tiers

The tiers communicated by the API are numeric. This is the mapping:

0. Reserved. It's not used by the API.
1. Free.
2. Premium 5.
3. Premium 20.
4. Premium 80.

## Auth endpoints

### POST `/login`

Sets the `skynet-jwt` cookie.

* Requires valid JWT: `true`
* GET params: none
* POST params: none
* Returns:
    - 204
    - 400
    - 401 (missing JWT)
    - 500

### POST `/logout`

Removes the `skynet-jwt` cookie.

* Requires valid JWT: `true`
* GET params: none
* POST params: none
* Returns:
    - 204
    - 400
    - 401 (missing JWT)
    - 500

## Skylink endpoints

### DELETE `/skylink/:skylink`

Deletes all uploads of this skylink by the current user.

* Requires valid JWT: `true`
* GET params:
    - skylink
* Returns:
    - 204
    - 401 (missing JWT)
    - 404 (when there is no such user. we won't try to create it)
    - 500 (on any other error)

## Stripe endpoints

### GET `/stripe/prices`

Returns the current list of subscription prices configured on Stripe.

```json
[
  {
    "id": "price_1IP7ddIzjULiPWN6vBhBe9EG",
    "name": "Skynet Extreme",
    "description": "Pin up to 20TB",
    "tier": 4,
    "price": 80,
    "currency": "usd",
    "stripe": "price_1IP7ddIzjULiPWN6vBhBe9EG",
    "productId": "prod_J19xoBYOMbSlq4",
    "livemode": true
  },
  {
    "id": "price_1IP7dMIzjULiPWN6YHoHM3hK",
    "name": "Skynet Pro",
    "description": "Pin up to 4TB",
    "tier": 3,
    "price": 20,
    "currency": "usd",
    "stripe": "price_1IP7dMIzjULiPWN6YHoHM3hK",
    "productId": "prod_J19xHMxmCmBScY",
    "livemode": true
  },
  {
    "id": "price_1IO6AdIzjULiPWN6PtviaWtS",
    "name": "Skynet Plus",
    "description": "Pin up to 1TB",
    "tier": 2,
    "price": 5,
    "currency": "usd",
    "stripe": "price_1IO6AdIzjULiPWN6PtviaWtS",
    "productId": "prod_J06NWykm9SRvWw",
    "livemode": true
  }
]
```

### POST `/stripe/webhook`

Exposed for communication with Stripe. Should not be called by anyone else. All requests which are not properly
authenticated will result in error.

## Tracking endpoints

### POST `/track/download/:skylink`

* Requires valid JWT: `true`
* GET params:
    - skylink: just the skylink hash, no path, no protocol
* POST params: none
* Returns:
    - 204
    - 400
    - 401 (missing JWT)
    - 500

### POST `/track/upload/:skylink`

* Requires valid JWT: `true`
* GET params:
    - skylink: just the skylink hash, no path, no protocol
* POST params: none
* Returns:
    - 204
    - 400
    - 401 (missing JWT)
    - 500

### POST `/track/registry/read`

* Requires valid JWT: `true`
* GET params: none
* POST params: none
* Returns:
    - 204
    - 400
    - 401 (missing JWT)
    - 500

### POST `/track/registry/write`

* Requires valid JWT: `true`
* GET params: none
* POST params: none
* Returns:
    - 204
    - 400
    - 401 (missing JWT)
    - 500

## User endpoints

### GET `/user`

This request combines the "get user data" and "create user" requests - if the users exists in the DB, their data will be
returned. If they don't exist in the DB, an account will be created on the Free tier.

* Requires valid JWT: `true`
* Returns:
    - 200 JSON object
  ```json
  {
    "email": "user@example.com",
    "sub": "5c972bbf-2e57-420a-850e-fde482bfdcce",
    "tier": 3,
    "subscribedUntil": "0001-01-01T00:00:00Z",
    "subscriptionStatus": "", // can also be "active"
    "subscriptionCancelAt": "0001-01-01T00:00:00Z",
    "subscriptionCancelAtPeriodEnd": false,
    "stripeCustomerId": "",
    "quotaExceeded": false
  }
  ```
    - 401 (missing JWT)
    - 424 (when there is no such user, and we fail to create it)
    - 500 (on any other error)

### PUT `/user`

This endpoint allows us to update the user's tier, membership expiration dates, etc.

* Requires valid JWT: `true`
* POST params:
    - None
* Body:
  ```json
    {
      "stripeCustomerId": "newStripeCustomerID"
    }
  ```
* Returns:
    - 200 JSON object, identical to `GET /user`
    - 400
    - 401 (missing JWT)
    - 500

### GET `/user/downloads`

Returns a list of all skylinks downloads by the user.

* Requires valid JWT: `true`
* Returns:
    - 200 JSON
      ```json
        {
          "items": [
            {
            "id": "606dc3c58e52495eb20467d6",
            "skylink": "HELLO__5MJyl1DKyfbuQMY_XBOBC9bR7idiU6isp6LXxEw",
            "name": "This is my HELLO skyfile!",
            "size": 132412341234,
            "rawStorage": 397284474880,
            "uploadedOn": "2021-04-07T14:37:57.586Z"
            }
          ],
          "offset": 0,
          "pageSize": 10,
          "count": 1
        }
      ```
    - 401 (missing JWT)
    - 424 (when there is no such user, and we fail to create it)
    - 500 (on any other error)

### GET `/user/limits`

Returns the speed limits that apply to this user. Those cover bandwidth, registry access delay, etc. These limits are
dynamic, as they are determined by both the current subscription of the user and whether they've exceeded any of their
quotas.

* Requires valid JWT: `true`
* Returns:
    - 200 JSON
  ```json
  {
    "upload": 5242880,
    "download": 20971520,
    "maxUploadSize": 1073741824,
    "registry": 0
  }
  ```
    - 401 (missing JWT)

### GET `/user/stats`

Returns statistics about the current user.

* Requires valid JWT: `true`
* Returns:
    - 200 JSON object
  ```json
  {
    "rawStorageUsed": 397284474880,
    "numRegReads": 0,
    "numRegWrites": 0,
    "numUploads": 5,
    "numDownloads": 0,
    "totalUploadsSize": 132412341234,
    "totalDownloadsSize": 0,
    "bwUploads": 1986422374400,
    "bwDownloads": 0,
    "bwRegReads": 0,
    "bwRegWrites": 0
  }
  ```
    - 401 (missing JWT)
    - 404 (when there is no such user. we won't try to create it)
    - 500 (on any other error)

### GET `/user/uploads`

Returns a list of all skylinks uploaded by the user.

* Requires valid JWT: `true`
* Returns:
    - 200 JSON
    ```json
      {
        "items": [
          {
            "id": "606dc3c58e52495eb20467d6",
            "skylink": "HELLO__5MJyl1DKyfbuQMY_XBOBC9bR7idiU6isp6LXxEw",
            "name": "This is my HELLO skyfile!",
            "size": 132412341234,
            "rawStorage": 397284474880,
            "uploadedOn": "2021-04-07T14:37:57.586Z"
          }
        ],
        "offset": 0,
        "pageSize": 10,
        "count": 1
      }
    ```
    - 401 (missing JWT)
    - 424 (when there is no such user, and we fail to create it)
    - 500 (on any other error)

### DELETE `/user/uploads/:uploadId`

Deletes a specific upload.

* Requires valid JWT: `true`
    - 204
    - 401 (missing JWT)
    - 404 (when there is no such user. we won't try to create it)
    - 500 (on any other error)
