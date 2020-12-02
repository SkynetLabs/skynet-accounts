# Basic API Guide

## Login

* Endpoint: `/login`
* Verb: `POST`
* Requires existing session: `false`
* POST params:
    - `email`: string
    - `password`: string
* Returns: 200 JSON string (JWT token)
```json
"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiNWZjNTFkOWNhMGYzNTFmYjYyMjkwMDlhIiwidGllciI6MSwiZXhwIjoxNjA2ODQwNjAxLCJpYXQiOjE2MDY3NTQyMDEsImlzcyI6IlNreW5ldCBBY2NvdW50cyJ9.0dhkOMHFBcoZSSBDrVBca5SOSeU3zMEOLVfNQWf_cOI"
```
also 400, 401, 422, 500

## User

### Create

* Endpoint: `/user`
* Verb: `POST`
* Requires existing session: `false`
* POST params:
    - `email`: string, must be valid, unique to the DB
    - `password`: string 
    - `firstName`: string, optional
    - `lastName`: string, optional
* Returns: 201 JSON object
```json
{
    "firstName": "John",
    "lastName": "Doe",
    "email": "john@doe.com",
    "tier": 0
}
```
also 400 and 500

### Lookup own user data

* Endpoint: `/user`
* Verb: `GET`
* Requires existing session: `true`
* Returns: 200 JSON object
```json
{
    "firstName": "John",
    "lastName": "Doe",
    "email": "john@doe.com",
    "tier": 1
}
```
also 400, 401, 500

### Update own user data

* Endpoint: `/user`
* Verb: `PUT`
* Requires existing session: `true`
* POST params:
    - `email`: string, optional, must be valid, unique to the DB
    - `firstName`: string, optional
    - `lastName`: string, optional
* Returns: 200 JSON object
```json
{
   "firstName": "John",
   "lastName": "Doe",
   "email": "john@doe.com",
   "tier": 1
}
```
also 400, 401, 500

## Password

### Change

* Endpoint: `/user/password`
* Verb: `POST`
* Requires existing session: `true`
* POST params:
    - `oldPassword`: string
    - `newPassword`: string
* Returns: 204 or 400, 401, 500
