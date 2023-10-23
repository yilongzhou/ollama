#!/bin/bash

cp -R app/Ollama.app $TMPDIR/Ollama.app && go build . && mv ./ollama $TMPDIR/Ollama.app/Contents/MacOS/Ollama && $TMPDIR/Ollama.app/Contents/MacOS/Ollama
