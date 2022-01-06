This folder contains the .proto file(s) and auto-generated files. Run `go generate` in this directory to generate the auto-generated go/twirp files.

Python client/definition files can be created like this:

```
go get github.com/twitchtv/twirp/protoc-gen-twirp_python (if not installed)
protoc  --twirp_python_out=. --python_out=. ./rpc/wordsearcher/searcher.proto
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
