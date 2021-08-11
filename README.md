### TODO ###

#### server ####
- [ ] SHA1 salt / hash?
- [x] dockerise go app
    - [x] push to github
    - [ ] write tests
    - [ ] set up CI
    - [x] set up CD (currently auto deploys w/o tests)
- [ ] update swagger for redirect endpoint
- [ ] project organization and structure
- [ ] validate and reject invalid request bodies i.e. invalid struct fields
- [ ] handle multiple errors / error handling. Look up Golang error handling.
- [ ] fix sql warning: uint64 values with high bit set are not supported
    - [ ] fix sql exception not caught and thrown, only being logged
- [ ] how to handle expired?

#### client ####
- todo in separate repo
- [ ] UI client to consume server
- [ ] user to provide original url
- [ ] user to call shortened url which shld redirect

- [ ] deploy to heroku or similar
