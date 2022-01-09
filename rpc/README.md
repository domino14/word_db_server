This folder contains the .proto file(s) and auto-generated files. Run `go generate` in this directory to generate the auto-generated go/twirp files.

Python client/definition files can be created like this:

```
go get -u github.com/verloop/twirpy/protoc-gen-twirpy (if not installed)
protoc  --twirpy_out=. --python_out=. ./rpc/wordsearcher/searcher.proto
```

JS files can be created like this:

JS twirp file

```
go get -u https://github.com/thechriswalker/protoc-gen-twirp_js (if not installed)
protoc --js_out=import_style=commonjs,binary:. --twirp_js_out=. ./rpc/wordsearcher/searcher.proto
```

Some example JSON requests for debugging (should use Protobuf in prod!):

```
curl -vvv -X POST localhost:8180/twirp/wordsearcher.QuestionSearcher/Search -H "Content-Type: application/json" -d '{"searchparams": [{"condition": 0, "stringvalue": {"value": "NWL18"}}, {"condition": 1, "minmax": {"min": 7, "max": 8}}]}'
```

A more complicated JSON body:

```
{
   "searchparams":[
      {
         "condition":0,
         "stringvalue":{
            "value":"NWL18"
         }
      },
      {
         "condition":1,
         "minmax":{
            "min":7,
            "max":8
         }
      },
      {
         "condition":13,
         "alphamap":{
            "values":{
               "ABC":{
                  "words":[
                     "CAB"
                  ]
               },
               "AIMN":{
                  "words":[
                     "MAIN",
                     "MINA"
                  ]
               }
            }
         }
      },
      {
         "condition":7,
         "stringarray":{
            "values":[
               "foo",
               "bar"
            ]
         }
      }
   ]
}

```
