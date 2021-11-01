Version Scheme
--------------
Skynet Accounts uses the following versioning scheme, vX.X.X
 - First Digit signifies a major (compatibility breaking) release
 - Second Digit signifies a major (non compatibility breaking) release
 - Third Digit signifies a minor or patch release

Version History
---------------

Latest:

## Oct 28, 2021:
### v0.1.3
**Key Updates**
- Add caching to the `/user/limits` endpoint.
- Add a GitHub Action that runs the tests on push. 
- Add infrastructure to send emails, e.g. for email confirmation, recovering an account after forgetting the password,
  etc.
- Add an endpoint where a user can delete their own account. 
- Add endpoints for email confirmation and account recovery.

**Other**
- Add integration tests covering the handlers.

## Oct 18, 2021:
### v0.1.2
**Other**
- Rename user uploads delete handler to match naming convention.

## August 9th, 2021:
### v0.1.0 
Initial versioned release.
