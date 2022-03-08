Version Scheme
--------------
Skynet Accounts uses the following versioning scheme, vX.X.X
 - First Digit signifies a major (compatibility breaking) release
 - Second Digit signifies a major (non compatibility breaking) release
 - Third Digit signifies a minor or patch release

Version History
---------------

Latest:

## Mar 8, 2022:
### v1.0.1
**Key Updates**
- Add support for base32 skylinks.
- Add the endpoints needed for challenge-response login and registration.
- Allow changing of user's password via the PUT /user endpoint
- Add database-backed configuration options.
- Add an option for disabling new account registrations.
- BREAKING: All non-GET handlers now read their parameters from the request's body JSON instead of the form.  
- Allow updating user's pubKey via two new endpoints - GET /user/pubkey/register and POST /user/pubkey/register. 
- Remove the `GET /user/recover` endpoint in favour of the new `POST /user/recover/request` endpoint.

## Oct 18, 2021:
### v0.1.2
**Other**
- Rename user uploads delete handler to match naming convention.

## August 9th, 2021:
### v0.1.0 
Initial versioned release.
