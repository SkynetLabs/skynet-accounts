# Basic API Guide

## User

### Get

This request combines the "get user data" and "create user" requests - if the
users exists in the DB, their data will be returned. If they don't exist in the
DB, an account will be created on the Free tier.

* Endpoint: `/user`
* Verb: `GET`
* Requires existing session: `true`
* Returns:
  - 200 JSON object
```json
{
    "tier": 1
}
```
  - 404 (when there is no such user and we fail to create it)
  - 500 (on any other error)

### Update own user data (TODO)

* Endpoint: `/user`
* Verb: `PUT`
* Requires existing session: `true`
* POST params:
    - TBD
* Returns:
  - 200 JSON object
```json
{
   "tier": 1
}
```
  - 400
  - 500

## Reports

### Report an upload (TODO)

* Endpoint: `/report/upload/:skylink`
* Verb: `POST`
* Requires existing session: `true`
* POST params: none
* Returns: 204 or 400, 401, 500

### Report a download (TODO)

* Endpoint: `/report/download/:skylink`
* Verb: `POST`
* Requires existing session: `true`
* POST params: none
* Returns: 204 or 400, 401, 500
