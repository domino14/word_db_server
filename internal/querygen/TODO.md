To move over to using this new query gen and word_db_server in general.

- [ ] Finish writing it (obvs#)
    - [x] Move webolith word db functionality over, including tests.
    - [x] Use generated Twirp Python code to build an API client in `webolith`
    - [ ] Remove all word db related stuff from `webolith` (after deploying Twirp client & this server)
    - [ ] add circleci
    - [ ] add to stack (in aerolith-infra and in kubernetes)
- [ ] Fix private `webolith-word-dbs` repo.
    Consider removing dbs from it and somehow caching it. Note that `macondo` and `webolith` use it for their tests; `webolith` should not be using it anymore after we move out the db-related stuff, and `macondo` should only use it for the word lists and not the actual .db files. Move it to private Github repo.
- [ ] Create an RPC endpoint for anagramming with `macondo`. To simplify deployment and dependencies, remove `macondo` from the stack.
    - [ ] Anagrammer can remain in that repo since it's very general and uses dawgs heavily.
    - [ ] Blank/Build challenges should move to this repo.
    - [ ] Build a twirp API for all of those functions in this repo
    - [ ] Remove JSONRPC API from `macondo` for those functions
    - [ ] Remove all `macondo` calls from `webolith`, replace with Twirp calls
- [ ] Make sure server is robust (ctrl + c, etc)
- [ ] Make sure blank challenges on demand work
    - [ ] test everything
- [ ] Write a script to compile proto and copy to subdirs
