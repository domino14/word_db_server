"""
Convert the raw osps(x)utf.txt file into something that can be read
by the definition maker.

The raw file has definitions that are minimal, and repeated words.

"""
import sys


def expand_definition(definition):
    if definition == '':
        return ''
    return ' ' + ' / '.join(definition)


def convert(filepath):
    with open(filepath) as f:
        corpus = f.read()

    lines = corpus.split('\n')
    actual_dict = {}
    for line in lines:
        splitword = line.split(None, 1)
        definition = ''
        if len(splitword) == 0:
            continue
        if len(splitword) == 2:
            word, definition = splitword
            definition = definition.strip()
            word = word.strip()
        else:
            word = splitword[0].strip()

        if word not in actual_dict:
            actual_dict[word] = [definition]
        else:
            actual_dict[word].append(definition)

    with open(filepath + '-out', 'w') as f:
        items = actual_dict.items()
        items = sorted(items, key=lambda item: item[0])
        for item in items:
            f.write(item[0] + expand_definition(item[1]))
            f.write('\n')


if __name__ == '__main__':
    convert(sys.argv[1])
