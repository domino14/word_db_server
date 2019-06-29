To move over to using this new query gen and word_db_server in general.

- [x] Finish writing it (obvs#)
    - [x] Move webolith word db functionality over, including tests.
    - [x] Use generated Twirp Python code to build an API client in `webolith`
    - [x] Remove all word db related stuff from `webolith` (after deploying Twirp client & this server)
    - [x] add circleci
    - [x] add to stack (in aerolith-infra and in kubernetes)
- [x] Fix private `webolith-word-dbs` repo.
    Consider removing dbs from it and somehow caching it. Note that `macondo` and `webolith` use it for their tests; `webolith` should not be using it anymore after we move out the db-related stuff, and `macondo` should only use it for the word lists and not the actual .db files. Move it to private Github repo.
- [x] Create an RPC endpoint for anagramming with `macondo`. To simplify deployment and dependencies, remove `macondo` from the stack.
    - [x] Anagrammer can remain in that repo since it's very general and uses dawgs heavily.
    - [x] Blank/Build challenges should move to this repo.
    - [x] Build a twirp API for all of those functions in this repo
    - [x] Add expand option to anagrammer
    - [x] Remove JSONRPC API from `macondo` for those functions
    - [x] Remove all `macondo` calls from `webolith`, replace with Twirp calls
- [x] Make sure server is robust (ctrl + c, etc)
    - [x] Check liveness probe carefully
- [x] Make sure context timeouts work with blank/build challs
- [x] Make sure blank challenges on demand work
    - [x] test everything
- [x] Write a script to compile proto and copy to subdirs
- [x] make sure to add word_db_server address to k8s file
- [x] blank challenge creator takes 30 seconds to time out and keeps repeating (instead of 5)
- [ ] deploy!