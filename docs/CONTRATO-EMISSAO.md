# O contrato de emissao, lido da fonte

O que um agente que consome `redigit emit` pode assumir. Cada ponto foi lido do
codigo, nao de memoria: uma sessao vizinha carregava estas quatro regras numa
anotacao derivada e errava duas delas, porque derivou em vez de ler.

Fonte de verdade: `internal/transport/cli/emit.go` (o DTO que sai),
`internal/domain/usecases/emitting/` (quem o produz).

## 1. O manifesto e aditivo, e nao ha campo `deleted`

Uma linha do manifesto carrega `position, subject, shape, feature, reason,
is_wip, chunks, blobs, risks`. Nao ha campo de remocao.

A garantia e mais forte do que "ninguem implementou": `emitting.Source.Read`
devolve o conteudo FINAL de um caminho, e todo blob e um render de um
subconjunto dele. Nada fora da arvore final pode nascer, entao remocao e
irrepresentavel, nao apenas ausente.

**Ausencia em `blobs[]` significa INALTERADO desde o ultimo commit que o
listou**, nunca apagado. `renderTouched` emite so os caminhos tocados; quem
escreve a historia carrega os demais adiante.

Uma aresta que morde: um arquivo cujas declaracoes ainda nao nasceram renderiza
vazio e e PULADO - exceto um que a arvore final guarda vazio, que e escrito,
porque o git o rastreia e pular deixaria a arvore com um caminho a menos.

## 2. Data e autor nao viajam

Nao ha campo de data nem de autor no manifesto. Uma varredura por
`author|committer|time.Time` em `internal/domain` e `internal/transport` volta
sem nenhuma ocorrencia fora de teste.

Nao existe "data deterministica por posicao", e o SHA **nao** e reproduzivel so
do manifesto. Determinismo e trabalho de quem escreve: `GIT_AUTHOR_DATE` e
`GIT_COMMITTER_DATE` explicitos por posicao. E o que o PLANO-EMISSAO secao 6 ja
dizia - a ferramenta nao escolhe emoji nem autoria.

## 3. O blob e lido por `handle`, e o `handle` depende do sink

O campo emitido chama-se `handle`. O valor muda com o destino:

| sink | handle | uso |
|---|---|---|
| banco de objetos (padrao) | oid git real, de `git hash-object -w` | `git cat-file blob <handle>` serve direto |
| `--blob-dir` (`DirSink`) | sha256 do conteudo cru, 64 hex | e o nome do arquivo no diretorio |

O sha256 do `DirSink` **nao** e um oid git: o git prefixa `blob <n>\0` e usa
sha1. Entao "handle difere de oid" vale so no modo diretorio. No modo padrao
handle e oid sao a mesma coisa.

A regra que o proprio `DirSink` enuncia: o `path` diz onde o conteudo PERTENCE,
o `handle` diz onde ele ESTA.

## 4. `chunks[].span` descreve o arquivo FINAL

Nao o blob daquele commit. `emitting.Chunk` e explicito: "positioned in the
FINAL file it belongs to". O agente tem a arvore final em maos, entao o span
basta para ver o corpo exato sem duplicar megabytes por commit.

## 5. O gemeo morto, e por que ele foi removido

Existia um segundo par `Commit`/`Blob` em `internal/infra/gitstore/manifest.go`,
chaveado em `oid`, com `EncodeManifest`/`DecodeManifest`, alcancavel apenas
pelos proprios testes. O doc dele colocava o span "in the blob it belongs to",
contradizendo o ponto 4.

Um parser escrito contra ele lia `null` em toda linha e parecia um manifesto
vazio em vez de um nome de campo errado. Foi deletado. **Leia `handle`.**

E a classe de defeito que mais apareceu por aqui: duas representacoes de uma
verdade, e a que a maquina le vence em silencio.

## 6. O ponto cego de build tag agora tem voz

Um arquivo atras de `//go:build integration` sem a tag passada responde a
consulta sintatica normalmente e fica invisivel a `references`. As declaracoes
chegam ao grafo SEM ARESTA NENHUMA, o que e indistinguivel de codigo que
ninguem le.

Isso nao e um `Blind`: aquele conta consulta que ERROU. Esta responde, e mente.

Hoje `Auditing.Unseen` nomeia esses arquivos, todo comando avisa em stderr, e
`redigit audit` os lista sob `unseen` com a restricao de cada um. A chave esta
sempre presente, inclusive vazia, para separar "nada foi escondido" de "a
ferramenta nem olhou".

Passe `--tags` deliberadamente. Sem isso o numero e sobre menos codigo do que
voce pensa.

## 7. A invariante da arvore e necessaria, nao suficiente

Registrado de uma reconstrucao vizinha de 300 commits, que encontrou isto ao
executar e nao ao raciocinar.

Duas execucoes bateram o hash da arvore alvo perfeitamente enquanto a historia
no meio estava errada nas duas:

- resolver conflito com a pos-imagem de um commit fonte tardio arrasta toda
  mudanca posterior daquele caminho, e esvaziou 12 commits anteriores que
  legitimamente entregavam algo ali;
- adiar um caminho que o HEAD nunca viu nao pode "restaurar a versao do HEAD",
  e um commit de trim de comentario acabou CRIANDO um arquivo de teste 74
  posicoes antes de quem deveria introduzi-lo.

A invariante e cega as duas. E a mesma coisa que o PLANO secao 5 diz sobre
replay e agrupamento serem duas metricas, alcancada pelo outro lado. Se o item
6 do roadmap (preservar a ordem publicada) for construido, ele reencontra estas
duas do mesmo jeito.
