# A ferramenta monta os pedacos, o agente faz os commits

Refinamento do desenho apos a decisao do dono. Substitui a secao "reescrita" do
PLANO.md; o resto daquele documento continua valendo.

## 1. O que muda, e por que e mais simples

Antes: `redigit` reconstruia a historia sozinho - planejava, renderizava,
escrevia commits com `fast-import` e conferia a invariante.

Agora: `redigit` para na **materializacao**. Ele decide o que entra em cada
commit, renderiza o conteudo de cada arquivo naquele ponto, e entrega os blobs
com um manifesto. Quem escreve a historia e o agente que orquestra, e e ele quem
valida, conserta e decide se divide mais.

Isso remove as duas pecas mais caras e mais arriscadas:

- **escrever git em escala**, que nunca foi provado alem de 2 commits de teste
- **julgar se o resultado esta certo**, que exige compilar, e compilar exige
  conserto, e conserto exige ler o codigo

E devolve a ferramenta o que ela ja faz bem: resolver simbolo com exatidao,
ordenar por dependencia, e cortar arquivo por declaracao.

A divisao fica: **a ferramenta responde ONDE cada declaracao nasce; o agente
responde se aquilo compila e o que fazer quando nao compila.**

## 1.1 O papel da ferramenta: pernas e bracos

A ferramenta nao tem julgamento e nao deve ganhar nenhum. Ela e precisao
mecanica a servico de um agente que tem contexto: resolve simbolo sem errar,
recorta por declaracao sem quebrar sintaxe, monta blob sem perder modo de
arquivo, e diz o que sabe que pode falhar.

O agente tem o que a ferramenta nao tem: o fonte completo, `go build`, `go vet`,
a suite, e a capacidade de ler um erro e consertar. Onde a ferramenta nao sabe
uma coisa, ela **pergunta ao agente** em vez de adivinhar - e a secao 2.2 e o
primeiro exemplo disso.

## 2. A ideia que unifica arquivo nao-fonte

Um arquivo sem declaracoes - `go.mod`, uma query `.sql`, um `.md` - hoje nao
entra no plano, porque o plano vem do grafo de declaracoes. Sao 237 arquivos na
arvore alvo, incluindo `go.mod` e `go.sum`, sem os quais nada compila.

**A solucao nao e maquinaria paralela: e tratar o arquivo nao-fonte com o mesmo
vocabulario.** Ja existe precedente - um `deps.go` que so tem clausula de pacote
e imports em branco recebe uma declaracao `KindOther` chamada `<file>`, com span
cobrindo o arquivo todo. Sem isso o arquivo sumia da arvore.

Mas **arquivo inteiro e o RECUO, nao a regra.** Um `.sql` de sqlc com vinte
queries e exatamente um `.go` com vinte funcoes: se o primeiro commit so usa
tres, so essas tres devem nascer. Ver a secao 2.1.

- todo arquivo nao-fonte vira uma ou mais declaracoes `KindOther`
- quando o formato tem estrutura conhecida, uma por pedaco; senao, uma so
- o pack fornece as arestas de cada uma
- dai em diante e tudo igual: mesmo grafo, mesmo ponto fixo, mesmo planejador

Nada de estagio novo. O arquivo nao-fonte vira cidadao da mesma ordenacao.

## 2.1 Recortar arquivo que nao e codigo

Uma porta `Chunker` por formato, com arquivo-inteiro como implementacao padrao.
Os que pagam o esforco, todos com precedente:

**`.sql` de sqlc** - cada bloco `-- name: Nome :one` e uma declaracao. O
comentario acima do bloco pertence a ele, pela mesma razao que o doc comment
pertence a funcao em Go: soltar um sem o outro deixa comentario descrevendo o
que nao esta la. O pack Go JA le essas linhas para ligar `dbgen` a query; hoje a
informacao e lida e jogada fora depois de servir a uma aresta.

O ganho e exato, nao cosmetico. Hoje o arquivo `.sql` inteiro tem de nascer
quando a PRIMEIRA funcao gerada precisa dele, arrastando dezenove queries que
ninguem le ainda. Com recorte, cada query nasce junto da funcao que ela gera.

**`go.mod`** - cada linha `require` e `replace` e uma declaracao, ligada ao
modulo que ela nomeia. Um modulo nao pode exigir um irmao que ainda nao existe,
e a linha e peso morto ate la. Ha precedente direto: a dobra anterior tinha um
passo `grow_modules` que fazia exatamente esse corte.

**`go.sum`** - cada par de linhas `modulo versao h1:` e `modulo versao/go.mod
h1:` e uma declaracao, ligada a linha `require` que nomeia aquele modulo naquela
versao. As duas linhas do par andam JUNTAS: o build precisa das duas.

Sem este recortador o de `go.mod` nao entrega ganho nenhum - o primeiro
`require` arrastaria o `go.sum` inteiro, e com ele o hash de toda dependencia
que ainda nao foi usada. Sao 26 arquivos e 668 linhas no repositorio medido.

**E ele erra em um so sentido, o que decide o desenho.** Uma linha a MAIS no
`go.sum` e inofensiva; uma linha a MENOS quebra `go build` com erro de hash
faltando. Entao a peca residual - as linhas transitivas que nao casam com
nenhum `require`, e nao ha `require` a que ligar - nasce CEDO, com o cabecalho
do modulo, e nao tarde. E o unico recortador onde o residuo vai para a frente
da fila em vez do fim, e a razao e que errar cedo aqui nao custa nada e errar
tarde quebra o build.

Isso tambem torna a garantia de "o que sobra vira peca propria" carregante
neste formato especifico: uma linha que suma silenciosamente nao aparece como
arquivo faltando, aparece como build quebrado varios commits depois.

**`go.work`** - cada linha `use`, ligada ao `go.mod` daquele diretorio. Mesmo
precedente (`grow_workspace`): o arquivo lista os 33 modulos desde o comeco
enquanto os `go.mod` chegam ao longo de 360 commits, e todo comando que le o
workspace falha ate o ultimo chegar.

**Onde arquivo inteiro continua certo**, e vale dizer para nao recortar por
recortar: uma migracao e atomica por definicao - metade de um `CREATE TABLE` nao
e nada. Um `.gitignore` ou um `LICENSE` nao tem consumidor por pedaco. Um `.md`
poderia ser cortado por secao, mas documento pela metade le pior que documento
ausente, e o dono ja manda prosa para o fim.

## 2.2 Quando a ferramenta nao conhece o formato: perguntar ao agente

Nao vamos escrever um recortador para todo formato que aparecer, e adivinhar
seria pior que nao recortar. Entao a ferramenta aceita um **mapa de recorte
externo**, que o agente fornece:

```json
{"database/queries/lead.sql": [
   {"name": "GetLeadByID",   "span": [0, 240]},
   {"name": "ListLeadsByOrg", "span": [241, 520]}
 ]}
```

A ferramenta trata cada entrada como declaracao: ordena, liga e renderiza. O
agente decidiu ONDE cortar; a ferramenta faz o corte sem errar um byte. E a
divisao de trabalho da secao 1.1 aplicada ao caso concreto - julgamento dele,
precisao nossa.

Duas garantias que a ferramenta da sobre o mapa, porque mapa errado e pior que
mapa ausente: os spans nao podem se sobrepor, e o que sobra fora deles vira uma
declaracao residual do arquivo, para nada sumir em silencio.

### As arestas que cada tipo precisa

Boa parte ja existe como `EdgeReason`:

| arquivo | regra | aresta |
|---|---|---|
| `go.mod`, `go.sum`, `go.work` | antes de qualquer declaracao do modulo | `hosts` |
| `database/queries/*.sql` | cada query antes da funcao gerada dela | `query` (existe) |
| linha `require`/`replace` | depois do `go.mod` do modulo que nomeia | `hosts` |
| linha `use` do `go.work` | depois do `go.mod` daquele diretorio | `hosts` |
| migracao | depois do schema, antes das queries | `schema` (existe) |
| schema `ent` | nao tem consumidor Go; nasce pelo que gera | `generated-by` (existe) |
| `testdata/`, fixture | com o teste que a le | `test-of` |
| `Taskfile`, config de modulo | com o modulo que serve | `hosts` |
| `.md` de pacote | commit proprio, junto do pacote | novo: `describes` |
| doc geral, `docs/**` | no fim da historia | novo: `prose` |
| CI, `.claude/skills`, `agents` | nos ultimos commits (regra 8 do dono) | novo: `prose` |
| `.gitignore`, LICENSE | primeiro commit | `hosts` |

Falta: `hosts` para manifesto, `describes` e `prose`. Tres motivos novos, nenhum
estagio novo.

## 3. O contrato: o que e um plano materializado

Por commit:

```
posicao        ordinal, denso, sem buraco
forma          feature | layer  (e a feature, quando houver)
motivo         por que este corte, em texto
chunks         as declaracoes que NASCEM aqui, em ordem topologica
blobs          por arquivo afetado: o conteudo COMPLETO naquele ponto
riscos         o que a ferramenta sabe que pode nao compilar
assunto        rascunho, marcado DRAFT:
```

Um `chunk` carrega chave (`arquivo:especie:nome`), o **span em bytes no arquivo
final**, e a unidade. O agente tem o fonte final em maos, entao span basta para
ele ver o corpo exato; o texto vai junto so quando pedido, porque duplicar 10 MB
por commit nao paga.

Um `blob` e o arquivo inteiro como fica DEPOIS deste commit - conteudo
acumulado, nao diff. E o que o `fast-import` e o `update-index` querem, e e o
que permite ao agente reescrever um arquivo sem reconstruir a historia.

### Riscos, que e o que o agente valida

A ferramenta sabe, sem compilar, quais referencias de um chunk apontam para
declaracao ainda nao nascida. Isso vai no manifesto:

```
riscos: [ {chunk, referencia, nasce_em}, ... ]
```

Um commit com lista vazia provavelmente compila. Um com lista cheia e candidato
a `[wip]`, ou a ser adiado. **A ferramenta nao decide qual dos dois** - ela
informa, e o agente, que tem `go build` e o fonte, decide.

## 4. A propriedade que torna "dividir mais" barato

Os chunks de um commit saem em **ordem topologica entre si**. Isso da ao agente
uma operacao barata: pegar o prefixo dos primeiros K chunks e re-renderizar.

Ressalva honesta: prefixo **nao garante** que compila. Uma declaracao pode
depender de outra que ainda nao nasceu em nenhum commit, e a propria historia
alvo aceita commit que nao compila, marcado `[wip]`. O que a ordem topologica
garante e mais modesto e ainda util: **um prefixo nunca quebra uma dependencia
que este mesmo commit ja satisfazia.** Dividir nunca piora o que estava dentro.

Para o agente exercer isso sem re-rodar o indice inteiro, a ferramenta expoe
`redigit render`: dado um arquivo e um conjunto de chaves, devolve o blob. E o
mesmo primitivo que o `declfold3` do repositorio vizinho ja tem, validado em
266 arquivos reais.

## 5. Formato de saida: identificadores git, nao conteudo embutido

Um JSON com o conteudo dentro seria enorme - a arvore tem ~10 MB de Go, e cada
commit reescreve varios arquivos.

**A ferramenta escreve os blobs no banco de objetos do repositorio** com
`git hash-object -w` e emite so os identificadores no manifesto:

```json
{"position": 41, "shape": "feature", "feature": "modules/lead",
 "chunks": [{"key":"modules/lead/lead.go:type:Lead","span":[120,340]}],
 "blobs":  [{"path":"modules/lead/lead.go","oid":"3f2a...","mode":"100644"}],
 "risks":  []}
```

Vantagens: sem duplicacao (git deduplica blob identico sozinho), manifesto
pequeno, e o agente usa `update-index --cacheinfo` direto, que e o maquinario
que ele ja tem. Escrever objeto solto e aditivo e coletavel por `git gc`; nao
mexe em ref, indice nem arvore de trabalho.

Alternativa para quem nao quer escrita: `--blobs=dir` grava
`plano/0041/<caminho>` em disco. Mais lento e duplicado, mas nao toca no repo.

**O modo de arquivo carrega junto** - `100644`, `100755`, `120000`. Um script
executavel reconstruido como arquivo comum da arvore diferente, e a invariante
falha no fim sem dizer por que. Isso ja mordeu uma vez aqui.

## 6. O que a ferramenta NAO faz

Deliberado, e vale escrever para nao voltar por engano:

- **nao decide a historia** - `emit` entrega o plano; quem escreve commits e o
  verbo `write`, a partir de um manifesto. Um manifesto sem edicao tambem
  escreve, com `--allow-draft`, e os assuntos saem como rascunho (T no
  157d218 escreve f750f82): o agrupamento automatico nunca chega sozinho a um
  commit sem esse flag
- **nao escolhe emoji nem autoria** - o emoji vem do assunto que o agente
  escreve, a autoria vem da configuracao
- **nao decide `[wip]`** - reporta risco e `is_wip`, o agente decide. Decidido
  o marcador, ele e verdade: `write` o recoloca mesmo quando o agente escreveu
  o assunto a mao, sem duplicar, no comeco da descricao, depois de `DRAFT: `,
  do `:emoji: ` e do `tipo(escopo): `, que e o lugar (o "slot") onde `verify`
  o le (`:bug: fix(b): [wip] Mao`). O tipo e letras minusculas e o `(escopo)`
  e opcional e pode ter espacos e `: ` (`fix(meu escopo): `); parentese sem
  fechar nao e tipo. Varios `[wip] ` no slot viram um. O marcador na posicao
  antiga, antes do tipo ou antes do emoji, conta como marcado e `write` o
  move para o slot. O mesmo texto em qualquer outro lugar
  da linha e palavra comum: nunca e apagado nem conta como marcador. O
  assunto e o que o git mostra em `%s` (linhas ate a primeira em branco,
  unidas por um espaco); `check`, `write` e `verify` leem esse mesmo texto.
  Sem isso um assunto a mao apagaria o `[wip]` e a linha passaria a contar
  como falha real
- **nao conserta import, comentario nem nada** - o renderizador poda import
  alcancavel, mas quem repara o que sobra e o agente, que tem o fonte
- **nao autoriza assunto** - emite rascunho marcado, e o marcador existe para
  o agente recusar o que nao foi revisado. `subjects check` recusa o que ainda
  e rascunho, o formato errado, assunto repetido (comparado sem o `[wip]`, ele
  e verdade da linha e nao do texto) e `[wip]` em linha que nao e
  wip
- **nao confia em assunto carregado de um plano velho** - `subjects carry`
  so move sem revisao o que tem o mesmo `id` (chaves de pedaco e caminhos de
  blob, sem spans); semelhanca de 0,8 ou mais e "review", e o resto volta a
  rascunho. O valor carregado nunca leva o `[wip]` (a verdade e da linha nova
  e `write` a aplica), e qualquer chave sem destino ou em conflito recusa como
  `check`, sem escrever nada

## 7. O que falta construir, em ordem

1. **Arquivo nao-fonte como declaracao** e as tres arestas novas (`hosts`,
   `describes`, `prose`). Sem isso o plano nao cobre 237 arquivos e nada
   compila. E o bloqueador.
1a. **Recortadores** de `.sql`, `go.mod` e `go.work`, mais o mapa externo do
   agente para o resto. Sem eles um `.sql` de vinte queries nasce inteiro na
   primeira, o que e a granularidade de ARQUIVO que a ferramenta toda existe
   para evitar.
2. **Materializacao**: renderizar por commit e escrever blobs, com modo de
   arquivo, emitindo o manifesto.
3. **`redigit emit`**: o comando, com `--blobs=git|dir`.
4. **`redigit render`**: o primitivo de recorte avulso, para o agente dividir
   sem re-indexar.
5. **Riscos por commit**: referencia que aponta para declaracao ainda nao
   nascida.
6. **Modo que preserva a ordem publicada**: hoje o planejador deriva ordem nova;
   o dono pediu reorganizar DENTRO da ordem que ja emergiu.

Os itens 1 a 3 fazem o caminho funcionar. O 4 e o 5 fazem o agente conseguir
trabalhar sem a ferramenta. O 6 e o que o dono pediu explicitamente e depende
dos anteriores.

## 8. A restricao que nao vem do grafo: `replace` precisa do diretorio

Descoberta pela sessao que reconstruiu `terectek_comms`, quebrando 53 arvores,
e registrada aqui porque o planejador desta ferramenta a reencontraria do mesmo
jeito.

Uma linha `replace github.com/org/lib => ../lib` num `go.mod` **nao pode nascer
antes do diretorio `../lib` existir**. O Go tenta ler o diretorio substituido
mesmo quando nada no modulo usa aquele import, entao o build quebra por uma
linha que ninguem exercita.

A consequencia e que essa restricao **nao vem do grafo de referencias**. Nenhuma
consulta ao language server a enxerga: nao ha simbolo, nao ha leitor, nao ha
aresta. Ela vem da existencia de um caminho no sistema de arquivos.

O que isso exige do pacote de regras:

- A peca de uma linha `replace` carrega o span da linha inteira, entao o
  construtor de arestas le o destino (`../lib`) dos proprios bytes. O
  recortador nao precisa expor o destino num campo.
- A aresta e da linha `replace` para **o que cria o diretorio destino** - na
  pratica, o `go.mod` daquele modulo, que e a primeira coisa a existir la.
- Quando uma linha `replace` tem varios destinos possiveis ao longo da
  historia, o piso e o **ultimo** deles a nascer, nao o primeiro: uma linha
  precisa de todos os seus destinos, nao de um.

Nao confundir com "o ponto em que o arquivo foi editado pela ultima vez". Foi
exatamente essa a tentativa que quebrou as 53 arvores: `libs/engines/flowrun`
nasce na posicao 131 e sete arquivos que o referenciam eram editados pela
ultima vez entre 78 e 117.

### Um sinal que caiu de graca

Corrigidos os pousos, os 9 pontos resultantes cairam **todos** em commits
`chore(...): Wire ... into the workspace`. Isso nao e coincidencia: se o pouso
correto de um `go.mod` e sempre um commit que ja mexe em `go.work`, entao o
commit de fiacao e o dono natural da linha `replace`. E uma regra que a
ferramenta pode aplicar em vez de um piso que ela precisa calcular, e o assunto
do commit ja descreve o que a linha faz.
