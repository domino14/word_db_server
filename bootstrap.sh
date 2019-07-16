#!/usr/bin/env bash
# This script "bootstraps" a container to fetch the lexicon files from git
# and create the databases/dawgs. See Dockerfile-bootstrap as well.

# Add more DBs here as Aerolith growz:  (or make this generic? maybe not)
SUPPORTED_LEXICA=("CSW15" "NWL18" "FISE2" "OSPS40")

# check if the lexicon path contains ALL of these. If it does not, we need to
# re-clone the repo and copy the text files over.
should_clone=0
for lexicon in $SUPPORTED_LEXICA
do
    if [ ! -f $LEXICON_PATH/$lexicon.txt ]; then
        should_clone=1
        break
    fi
done

if [ $should_clone == 1 ]; then
    git clone https://$GITHUB_TOKEN@github.com/domino14/word-game-lexica /tmp/word-game-lexica
    rm -rf /tmp/word-game-lexica/.git
    mv *.txt $LEXICON_PATH
fi

# Figure out which DATABASES are missing.

declare -a dbs_to_create

for lexicon in $SUPPORTED_LEXICA
do
    if [ ! -f $LEXICON_PATH/db/$lexicon.db ]; then
        dbs_to_create+=("$lexicon")
    fi
done

function join_by { local IFS="$1"; shift ; echo "$*"; }

dbs_str=$(join_by , "${dbs_to_create[@]}")
./cmd/dbmaker/dbmaker -outputdir $LEXICON_PATH/db -dbs $dbs_str

