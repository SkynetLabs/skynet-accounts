# API Guide

## General terms

### User tiers

The tiers communicated by the API are numeric. This is the mapping:

0. Reserved. It's not used by the API.
1. Free.
2. Premium 5.
3. Premium 20.
4. Premium 80.

## Health

### GET `/health`

Returns the health of the service

* Requires a valid JWT: `false`
* Returns:
 - 200 JSON object
  ```json
  {
    "dbAlive": "true"
  }
  ```

## Auth endpoints

### POST `/login`

Sets the `skynet-jwt` cookie.

* Requires valid JWT: `true`
* POST params: `email`, `password`
* Returns:
  - 204
  - 400
  - 401 (missing JWT)
  - 500

### POST `/logout`

Removes the `skynet-jwt` cookie.

* Requires valid JWT: `true`
* Returns:
  - 204
  - 400
  - 401 (missing JWT)
  - 500

## User endpoints

### POST `/user`

Creates a new user.

* Requires a valid JWT: `false`
* POST params: `email`, `password`
* Returns:
  - 200 JSON object - the user object
  - 400 (invalid email, missing password, email already used)
  - 500

### GET `/user`

This request combines the "get user data" and "create user" requests - if the users exists in the DB, their data will be
returned. If they don't exist in the DB, an account will be created on the Free tier.

* Requires valid JWT: `true`
* Returns:
  - 200 JSON object - the user object
  - 401 (missing JWT)
  - 404 (when there is no such user, and we fail to create it)
  - 500 (on any other error)

### PUT `/user`

This endpoint allows us to update the user's email or set their StripeID. If the
user's StripeID is already set and you try to update it you will get a 409 
Conflict.

* POST params:
  - JSON object (all fields are optional)
    ```json
    {
      "email": "user@siasky.net",
      "stripeCustomerId": "someStripeId"
    }
    ```

* Requires valid JWT: `true`
* Returns:
  - 200 JSON object - the user object
  - 400
  - 401 (missing JWT)
  - 404
  - 409 Conflict (StripeID is already set)
  - 500

### DELETE `/user`

Deletes the user and all of their data.

* Requires valid JWT: `true`
* Returns:
  - 204
  - 401 (missing JWT)
  - 404 (when there is no such user)
  - 500 (on any other error)

### GET `/user/limits`

Returns the portal limits of the current user. Returns the values for 
`anonymous` if there is no valid JWT.

* Requires a valid JWT: `false`
* Returns:
 - 200 JSON object
  ```json
  {
    "tierName": "anonymous",
    "upload": 123,
    "download": 123,
    "maxUploadSize": 123,
    "registry": 123
  }
  ```

### GET `/user/stats`

Returns statistical information about the user.

* Requires a valid JWT: `true`
* Returns:
 - 200 JSON object
  ```json
  {
    "rawStorageUsed": 123,
    "numRegReads": 123,
    "numRegWrites": 123,
    "numUploads": 123,
    "numDownloads": 123,
    "totalUploadsSize": 123,
    "totalDownloadsSize": 123,
    "bwUploads": 123,
    "bwDownloads":  123,
    "bwRegReads": 123,
    "bwRegWrites":  123
  }
  ```
 - 401
 - 404
 - 500

### GET `/user/uploads`

Returns a list of all skylinks uploaded by the user.

* Requires valid JWT: `true`
* Returns:
  - 200 JSON Array (TBD)
  - 401 (missing JWT)
  - 424 (when there is no such user, and we fail to create it)
  - 500 (on any other error)

### DELETE `/user/uploads/:skylink`

Deletes all uploads of this skylink made by the current user.

* Requires a valid JWT: `true`
* Returns:
 - 204
 - 400 (invalid skylink)
 - 401
 - 500

### GET `/user/downloads`

Returns a list of all skylinks downloads by the user.

* Requires valid JWT: `true`
* Returns:
  - 200 JSON Array (TBD)
  - 401 (missing JWT)
  - 424 (when there is no such user, and we fail to create it)
  - 500 (on any other error)

### GET `/user/confirm`

Validates the given `token` against the database and marks the respective email 
address as confirmed.

* Requires a valid JWT token: `false`
* GET params: `token`
* Returns:
- 200
- 400
- 500

### POST `/user/reconfirm`

Requests another confirmation email sent to the account's email address.

* Requires a valid JWT token: `true`
* Returns:
 - 204
 - 401
 - 500

### POST `/user/recover/request`

Requests a recovery token to be sent to given email. The email needs to be 
confirmed for the action to be performed.

* Requires a valid JWT token: `false`
* POST params: `email`
* Returns:
- 204
- 400
- 500

### POST `/user/recover`

Changes the user's password without them being logged in.

* Requires a valid JWT token: `false`
* POST params: `token`, `password`, `confirmPassword`
* Returns:
- 200
- 400
- 500

## API Keys endpoints

### PATCH `/user/apikeys/:id`

Updates the list of skylinks covered by a public API key.
Additions are performed before removals. Only one copy of each API key is stored.

* Requires valid JWT: `true`
* GET params: none
* Body:
```json
{
  "add": ["AADDE7_5MJyl1DKyfbuQMY_XBOBC9bR7idiU6isp6LXxEw", "AADDE7_5MJyl1DKyfbuQMY_XBOBC9bR7idiU6isp6LXxEw"],
  "remove": ["AADDE7_5MJyl1DKyfbuQMY_XBOBC9bR7idiU6isp6LXxEw", "AADDE7_5MJyl1DKyfbuQMY_XBOBC9bR7idiU6isp6LXxEw"]
}
```
* Returns:
- 204
- 400
- 401
- 500

### POST `/user/apikeys`

Creates a new general API key.
This type of API key gives full access to `accounts` and is equivalent to using a JWT token.
This type of API key needs to be kept secret and never be shared with anyone.

* Requires valid JWT: `true`
* GET params: none
* Body:
```json
{
  "name": "key's name",
  "public": "true",
  // The skylinks field is only applicable to public API keys. 
  "skylinks": ["AADDE7_5MJyl1DKyfbuQMY_XBOBC9bR7idiU6isp6LXxEw", "AADDE7_5MJyl1DKyfbuQMY_XBOBC9bR7idiU6isp6LXxEw"]
}
```
* Returns:
- 200
```json
{
  "id": "6221f3f248c7d376e12f99c4",
  "createdAt": "2022-03-04T11:11:46.946334Z",
  "key": "rpfccs5kLCib4PPERtcaY88_yHsJFNNpeMc62pYhBfM="
}
```
- 400
- 401
- 500

### PUT `/user/apikeys/:id`

Updates an API key.
Private API keys cannot be updated.
A public API key cannot be converted to private and vice-versa.

* Requires valid JWT: `true`
* GET params: none
* Body:
```json
{
  "skylinks": ["AADDE7_5MJyl1DKyfbuQMY_XBOBC9bR7idiU6isp6LXxEw", "AADDE7_5MJyl1DKyfbuQMY_XBOBC9bR7idiU6isp6LXxEw"]
}
```
* Returns:
- 204
- 400
- 401
- 500

### GET `/user/apikeys`

Lists all API keys registered by the current user.

Note: The actual API key will not be revealed, only its metadata.

* Requires valid JWT: `true`
* GET params: none
* Returns:
- 200
```json
[
    {
        "id": "620ba9c66e18552db39cd5ce",
        "createdAt": "2022-02-15T13:25:26.348Z"
    },
    {
        "id": "6221f3f248c7d376e12f99c4",
        "createdAt": "2022-03-04T11:11:46.946Z"
    }
]
```
- 401
- 500

### DELETE `/user/apikeys/:id`

Deletes the API key with the given ID.

* Requires valid JWT: `true`
* GET params: none
* Returns:
- 204
- 400
- 401
- 500

## Reports endpoints

### POST `/track/upload/:skylink`

* Requires valid JWT: `true`
* GET params:
  - skylink: just the skylink hash, no path, no protocol
* Returns:
  - 204
  - 400
  - 401 (missing JWT)
  - 500

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
