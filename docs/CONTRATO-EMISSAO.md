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

## 8. Ancoras: o commit de mudanca, `Changes` e `Versions`

So existe com `--mode lineage` e ancoras em `lineage.anchors`. Sem ancora, plano,
manifesto e blobs saem identicos byte a byte aos de antes.

Uma ancora e um commit `fix` ou `perf` da fonte que continua sendo um commit de
verdade. Cada declaracao que ele mudou nasce com o texto do PAI da ancora (a
pre-imagem) e vira o texto FINAL no commit da ancora.

- `PlannedCommit.Changes` lista as declaracoes nascidas num commit anterior que
  este reescreve da pre-imagem para o texto final. O assunto e o da fonte, sem
  `DRAFT`. O commit da ancora so tem `Decls` para o que a propria ancora fez
  nascer (ver adiante); fora isso, so `Changes`.
- `Plan.Versions[chave]` guarda as pre-imagens em ordem de `Until`. `Until` e a
  posicao do commit onde a proxima versao, ou o texto final, assume, e esse
  commit tem a chave em `Changes`: o emissor recusa o plano que viola isso.
  `PreimageAt(chave, p)` devolve a primeira versao com `p < Until`.
- Um commit que nasce antes da ancora renderiza a declaracao com a pre-imagem. O
  da ancora renderiza o texto final. O planejador compara a declaracao com todo o
  espaco em branco removido, no pai contra a ancora e na pre-imagem contra o
  final: uma ancora que so reformata nao muda declaracao nenhuma e e descartada,
  entao nao sai commit vazio. O emissor, por si, nao recusa um commit sem blobs.
- Uma declaracao com pre-imagem ativa nao entra na conta de referencias para
  frente: a pre-imagem nao e o texto final, e as arestas do grafo dizem respeito
  a este.
- Cada arquivo costurado passa pelo `Renderer` da linguagem, com todas as suas
  declaracoes, antes de a sonda compilar, como o emissor faz: uma pre-imagem que
  deixa de usar um import nao quebra a arvore por causa dele.
- Toda declaracao que a ancora fez nascer vai para as `Decls` do commit da ancora,
  onde quer que o plano a tivesse posto, para o commit ler como a correcao real:
  traz as declaracoes novas e o teste de regressao. Ela fica onde estava, com o
  aviso `anchors: born declaration stays`, se (a) algum provedor dela nasce depois
  e nao se move, fora o que so a preenche como interface; (b) a pre-imagem em
  vigor no ponto desta ancora (as das ancoras ate esta, mais a primeira entrada
  mais nova de cada chave que esta ancora nao tem) de uma declaracao ja colocada antes do
  commit da ancora cita o nome dela (`\bNome\b`; num metodo, o que vem depois do
  ultimo ponto), porque o grafo e o do texto final e nao tem essa aresta; a
  propria declaracao e as que se movem nao contam, e um metodo cujo tipo
  receptor se move nao passa por esta porta; ou (c) quem a le ja nasceu antes e
  nao se move, nao e entrada desta ancora nem de uma mais nova e nao so a
  preenche. As ancoras sao adotadas da mais nova para a mais antiga, depois que
  todas tem lugar, para que o consumidor que uma mais nova move depois nao
  segure. Uma declaracao que uma ancora ate esta tambem muda (apagada e posta de
  volta por esta) nunca se move, com o aviso `anchors: born declaration stays`,
  porque o commit da mais antiga a muda antes de a nova a fazer nascer. As que
  ficam e as que vao sao decididas juntas, ate nada mais mudar.
  Uma ancora que perdeu a maior parte da mudanca continua como commit de verdade
  sob o seu assunto (sempre ha um jeito de seguir); o resumo (`anchors: summary`)
  traz `moved` e `stayed` por ancora, e uma ancora quase oca fica visivel ali.
  Declaracoes do mesmo `Atom` vao juntas
  ou ficam todas. Sai dos outros commits o que se moveu; os assuntos `DRAFT` desses
  commits sao refeitos, e o que ficou vazio some depois que todas as ancoras
  foram postas. Uma ancora que so faz nascer declaracoes e um commit so de
  `Decls` quando algo se move para ele, senao e descartada.
- A pre-imagem so se aplica se ela compila. Uma sonda monta a arvore final com as
  pre-imagens costuradas e decide: `fold` descarta a ancora e `wip` a mantem e
  marca com `[wip]` todo commit entre o nascimento da pre-imagem e a ancora. O
  padrao e `fold`; `stop` chega na 24b e hoje so diz isso. A sonda so ve a arvore
  final, entao uma pre-imagem que le algo colocado depois dela nao e pega por
  ela. Depois de colocar e adotar as ancoras, cada entrada viva de uma ancora
  que compilou (`kept`; uma `wip` ja esta marcada e uma sem veredito continua sem
  veredito) e lida: se cita uma funcao, tipo ou valor da mesma unidade (um que e
  de teste, ou que esta num `_test.go`, so conta para quem tambem e de teste), ou
  por `pacote.Nome` (o ultimo trecho do diretorio) o de outra, colocado depois da
  ativacao (o lugar da ancora anterior da chave, ou o commit onde ela nasce), o
  veredito e de reprovada: `fold`, ou `wip` sob essa politica, com a declaracao
  no motivo. Uma entrada que nao e declaracao Go (um `README.md`) nao le nada.
  Os nomes lidos vem de uma analise sintatica quando a porta `Reader` esta ligada
  e o arquivo e Go: sao os identificadores que o texto da pre-imagem nao resolve
  sozinho, a chave solta de um literal composto que nao e um local nem um
  parametro (sem tipos pode ser um campo ou uma leitura) e `qualificador.Nome`
  quando o qualificador nao se resolve; um local, um parametro, um campo lido
  por seletor, um comentario ou um literal de texto nao contam. Sem a porta, ou
  quando o texto nao analisa sozinho (uma instrucao de funcao cortada, `Nome#i`),
  vale a busca da palavra inteira `Nome`, em que letra ou digito de qualquer
  alfabeto continua a palavra, no texto, comentarios e literais incluidos, e ela
  pode descartar uma ancora sa.
  A decisao se repete, uma ancora a mais por rodada (termina em no maximo uma
  rodada por ancora), porque descartar uma muda o lugar das outras; as sondas
  ja feitas nao se repetem. A mesma leitura, de dentro de `holds`, faz uma
  declaracao nascida cuja primeira pre-imagem mais nova leria algo colocado depois
  do commit da ancora ficar onde esta. Onde a leitura nao sabe dizer, o nome conta
  como lido: um falso positivo descarta a ancora sob `fold` ou a marca sob `wip`,
  e um falso negativo quebraria um commit; por isso a chave de um literal com o
  nome de uma declaracao posterior conta como lida, e um campo de struct pode
  descartar uma ancora que compilaria. Limites: um metodo chamado antes de
  nascer nao e visto, nem a leitura por um import com apelido, por um import com
  ponto (`import . "x"`) ou por um pacote cujo nome difere do diretorio (o da raiz
  do modulo incluido); o `verify` do plano ainda pega esses casos.
- Uma pre-imagem que nao pode ser costurada nem renderizada (um `go.mod` que a
  correcao transformou em bloco, uma casca com o mesmo trecho em duas entradas)
  nao pode ser emitida: o veredito e "inadequada" e a ancora cai em `fold` mesmo
  sob `wip`. Uma edicao fora da arvore e uma falha de leitura continuam sendo
  erro. Entradas e edicoes que dividem o mesmo trecho com o mesmo texto (um grupo
  `const (...)`) valem uma so; o mesmo trecho com textos diferentes
  (`var a = 1; var b = 2` virando `var a, b = 3, 4`) e inadequado e a ancora cai
  em `fold`, para a sonda e o emissor concordarem.
- A sonda de uma ancora mais antiga leva so as pre-imagens das ancoras mais novas
  que ja compilaram. Antes de editar, cada unidade afetada e compilada uma vez na
  arvore intacta. Uma unidade vermelha antes de qualquer edicao e compilada de
  novo depois dela: uma linha nova de diagnostico reprova a ancora (o caminho
  do checkout, o diretorio de trabalho do `go build` e a linha e coluna de `arquivo:linha:col:` nao contam); saida
  igual, como a de uma unidade que nem carrega (um `replace` para um diretorio
  ignorado pelo git), a deixa sem veredito, com o motivo.
- Sem veredito, a ancora fica como commit de verdade, sem `fold` e sem `[wip]`:
  unidade que ja falha antes de qualquer edicao, estouro de tempo, nenhuma unidade
  cobrindo as edicoes (um modulo na raiz cobre todo arquivo, entao uma entrada que
  nao e Go dentro dele fica `kept` com o motivo "its pre-image builds" sem que o
  build a leia: M6) ou arvore que nao se prepara (falha de ambiente: o veredito
  e o mesmo e fica para todas as ancoras, uma linha por ancora). So o cancelamento
  da execucao encerra o plano. O aviso traz o motivo. Ler algo colocado depois
  nao muda isso: uma ancora sem veredito nao vira `fold` nem `[wip]` por isso.
- Uma declaracao que a ancora mudou e nao pode ser costurada (renomeada ou apagada
  depois, de volta a pre-imagem, cortada em instrucoes) sai da ancora com um
  aviso. Um arquivo cuja revisao (a do pai ou a da ancora) nao analisa tambem e
  pulado, com o aviso `anchors: not spliced`. Mover o arquivo depois, com o mesmo
  tipo e nome, mantem a entrada na chave final; renomear a declaracao a perde.
  Uma ancora que fica sem nenhuma declaracao e descartada.
- Uma pre-imagem igual a seguinte da mesma declaracao, ate no espaco em branco, nao muda nada naquele
  commit e sai do plano; a ancora que fica sem mudanca nenhuma nunca vira commit
  vazio. Espaco em branco dentro de um literal de texto conta como sem mudanca
  (aceito).
- Cada ancora deixa uma linha de resumo em `Warn` (`anchors: summary`) com a
  revisao, o resultado (`kept`, `folded`, `wip`, `unchecked`, `dropped`), o motivo
  (a primeira linha do compilador que nao e cabecalho `#`), as declaracoes, as
  puladas e, quando a ancora vai ao plano, `slot`, `moved`, `stayed` e `marked`
  (os commits que o `wip` marcou para ela: a conta e por ancora e se sobrepoe, a
  soma passa o numero de commits marcados). Uma ancora `folded` tem antes uma linha
  em `Info` por declaracao
  (`anchors: folded declaration`).
- A sonda recebe o contexto da execucao: interromper o plano interrompe a sonda.
- `--mode history` tambem resolve as ancoras: uma revisao desconhecida falha, e
  as ancoras dadas fora do `--mode lineage` saem num aviso e sao ignoradas.
- O plano mostra `changes` em cada commit de ancora (omitido quando vazio).
- Um plano lido do cache (`--from-cache`) perde o relatorio das ancoras; o plano,
  o manifesto e os blobs sao os mesmos.
- Limites conhecidos: a pre-imagem de uma declaracao que usava um import que a
  correcao removeu por completo nao leva o import do pai, e a ancora pode cair em
  `fold` por isso (M2); a sonda compila com o padrao Go e as `tags`, nao com a
  configuracao de `verify`, e uma correcao fora de Go e de qualquer modulo fica
  sem unidade, logo sem veredito, e dentro de um modulo na raiz fica `kept` sem
  build que a leia (M6); o estouro de tempo e o campo `Outcome.IsTimeout`, que o
  cache de `verify` nao guarda; uma ancora que perdeu quase toda a mudanca por
  declaracoes que ficaram continua sob o assunto da correcao (ver acima); e uma
  unidade vermelha antes de qualquer edicao so reprova a ancora por uma linha nova de
  diagnostico, entao a mesma mensagem num outro ponto do arquivo (outra linha ou
  coluna) passa despercebida; e o espaco em branco e comparado removendo-o, sem
  tokenizador, entao dois textos que so diferem num espaco entre palavras (`gof()`
  e `go f()`) parecem iguais e a mudanca some sem aviso.

A invariante da arvore continua valendo: o ultimo commit tem a arvore do HEAD.
`[wip]` e tolerado pelo `verify`; um commit vermelho sem a marca nao.

## 9. O que a regra `go-router` promete e o que nao promete

Promete: uma funcao de montagem e a primeira instrucao que a chama sao um atomo
somente quando nem essa instrucao, nem o atomo que o fatiador lhe deu, nem o
grupo de recurso dela hospedam outra montagem da mesma regra (so uma instrucao
conta como chamador; uma funcao inteira nasce uma vez e juntaria todas as
montagens a ela;
tres montagens numa unica instrucao seguem separadas, porque unidas prenderiam
o corpo de uma ao chamador da outra). As instrucoes de um recurso podem formar
um atomo, e o grupo junta caminhos iguais, prefixo incluido, de grupos diferentes
na mesma funcao.
Duas regras `go-router` nunca unem atomos pelo rotulo: cada regra depois da
primeira tem o seu. Uma declaracao em duas regras fica no atomo da ultima; o
atomo da anterior a perde.

Os padroes de `handle` e `mount` casam o nome curto da chamada (`Router.GET`,
`Get`) ou o qualificado (`github.com/go-fuego/fuego.Get`). Um metodo tem o nome
do tipo que o declara, sem ponteiro e sem argumentos de tipo, e `*` nao
atravessa `/`: `*.Get` nao casa aquela funcao. Quando nenhum padrao de `handle`
casa chamada alguma, `Excluded` diz isso.

Nao promete ler rotas registradas por variavel de funcao, grupos abertos por
atribuicao ou por chamada encadeada, montagens passadas por variavel, nem rotas
de um modulo aninhado (um `go.mod` abaixo da raiz): o indice de fatos nao as
carrega, e a `go-di` divide o mesmo limite. O portao de rotas declara isso em
`Excluded` e nao conta o que nao ve.

O que o portao conta: so entra no total a rota com caminho constante e handler
nomeado; rota sem caminho ou sem handler fica fora, com a contagem dita. A
repeticao vale dentro de uma funcao; a mesma chave em outra funcao e dita e nao
julgada, e `HANDLEFUNC` e `HANDLE` contam como dois metodos. `Site` pode ser a
funcao inteira quando ela nao foi cortada em instrucoes, e `Path` ja vem unido
ao prefixo do grupo. `Blind` conta toda rota sem caminho, a constante que nao e
caminho inclusive (o portao a dita a parte), e `Inline` o handler que nenhum
fato nomeia; as duas ficam fora do total. Montagem chamada so por funcao inteira
aparece em `Excluded` como nao comparada. "Colocado" compara posicoes do
ordenamento, nao os commits do plano: uma montagem e seu chamador numa mesma
posicao podem cair em commits diferentes.
