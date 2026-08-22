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

## 2. A ideia que unifica arquivo nao-fonte

Um arquivo sem declaracoes - `go.mod`, uma query `.sql`, um `.md` - hoje nao
entra no plano, porque o plano vem do grafo de declaracoes. Sao 237 arquivos na
arvore alvo, incluindo `go.mod` e `go.sum`, sem os quais nada compila.

**A solucao nao e maquinaria paralela: e tratar o arquivo inteiro como UMA
declaracao.** Ja existe precedente no codigo - um `deps.go` que so tem clausula
de pacote e imports em branco ja recebe uma declaracao `KindOther` chamada
`<file>`, com span cobrindo o arquivo todo. Sem isso o arquivo sumia da arvore.

Estendendo a mesma ideia:

- todo arquivo nao-fonte recebe uma declaracao `KindOther`, span = arquivo todo
- o pack de linguagem (ou um pack de dados) fornece as arestas dela
- dai em diante e tudo igual: mesmo grafo, mesmo ponto fixo, mesmo planejador

Nada de estagio novo. O arquivo nao-fonte vira cidadao da mesma ordenacao.

### As arestas que cada tipo precisa

Boa parte ja existe como `EdgeReason`:

| arquivo | regra | aresta |
|---|---|---|
| `go.mod`, `go.sum`, `go.work` | antes de qualquer declaracao do modulo | `hosts` |
| `database/queries/*.sql` | antes do codigo gerado a partir dela | `query` (existe) |
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

- **nao escreve commit, ref nem historia**
- **nao escolhe emoji nem autoria**
- **nao decide `[wip]`** - reporta risco, o agente decide
- **nao conserta import, comentario nem nada** - o renderizador poda import
  alcancavel, mas quem repara o que sobra e o agente, que tem o fonte
- **nao autoriza assunto** - emite rascunho marcado, e o marcador existe para
  o agente recusar o que nao foi revisado

## 7. O que falta construir, em ordem

1. **Arquivo nao-fonte como declaracao** e as tres arestas novas (`hosts`,
   `describes`, `prose`). Sem isso o plano nao cobre 237 arquivos e nada
   compila. E o bloqueador.
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
