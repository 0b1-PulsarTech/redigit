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

Um bloco `const` com especificacao implicita (um nome sem tipo nem valor, como
o iota exige) e UM so `Atom`: seus nomes nascem juntos, na mesma linha, e o
renderizador entrega o bloco inteiro. Sem isso, o primeiro nome renderizava o
bloco todo antes de as constantes que ele le existirem (medido com `iota + C01`:
as linhas 1 e 2 nao compilavam) e um nome posterior caia numa linha de blob igual
ao do pai (arvore igual a do pai).

Duas regras da mesma natureza. Uma especificacao que liga mais de um nome
(`const A, B = 1, C01`, `var X, Y = 1, f()`) e UM so `Atom`: o renderizador
entrega a especificacao inteira se um nome sobrevive, entao os nomes tem de
nascer juntos. Um bloco `const` em que uma especificacao explicita, depois da
primeira, le `iota` e UM so `Atom`, como o implicito: `KeyB = iota` sozinho
vale 0 e nao 1. Fora esses casos, valores explicitos continuam um nome por
declaracao, sem `Atom`. O rotulo e `const-group:` (bloco) ou `spec-group:`
(especificacao) mais o arquivo relativo e a posicao fisica (uma diretiva
`//line` nao conta), sem tempo, ordem de mapa nem caminho absoluto. As linhas de
membros que dividem um `Span` contam uma vez, no primeiro membro na ordem das
declaracoes, para a conta do teto de um commit.

Limite: um `Atom` que uma cadeia de leitores le de volta fecha um ciclo no
grafo, mesmo quando a linguagem nao o tem (`KeyA = iota + C01` e `KeyB = iota`,
com `C01` lendo `KeyB` ao fim de uma cadeia). O ordenador avisa (`ordering holds
1 cycle`) e o plano sai; o caso medido sai verde.

Se todos os blobs de uma linha forem iguais ao handle
que o mesmo caminho tinha na ultima linha anterior, o emissor emite um
`slog.Warn` com a posicao e as chaves dos chunks; nao recusa nem omite a linha.
Um caminho visto pela primeira vez ou um handle diferente nao avisam, e o
manifesto nao muda.

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

Um arquivo atras de `//go:build integration` sem a tag passada fica fora do
build principal e responde a consulta sintatica normalmente. Com o resolvedor
`typed` (o padrao) passes extras o checam e, se ele compila, responde a
`references`; com `--resolver lsp` o servidor pode nao o enxergar, e suas
declaracoes chegam ao grafo SEM ARESTA NENHUMA, o que e indistinguivel de
codigo que ninguem le.

Isso nao e um `Blind`: aquele conta consulta que ERROU. Esta responde, e pode
mentir.

Hoje `Auditing.Unseen` nomeia esses arquivos, todo comando avisa em stderr, e
`redigit audit` os lista sob `unseen` com a restricao de cada um. A chave esta
sempre presente, inclusive vazia, para separar "nada foi escondido" de "a
ferramenta nem olhou".

Passe `--tags` deliberadamente. Sem isso o grafo pode ter menos arestas do que
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
  entao nao sai commit vazio. O emissor recusa um commit sem blobs
  (`ErrIncomplete`). Uma linha cujos blobs repetem o ultimo handle de cada
  caminho (e nao os da linha anterior) passa: so avisa (secao 1).
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
  padrao e `fold`; `stop` para e espera uma pessoa (abaixo). A sonda so ve a arvore
  final, entao uma pre-imagem que le algo colocado depois dela nao e pega por
  ela. Depois de colocar e adotar as ancoras, cada entrada viva de uma ancora
  que compilou (`kept`; uma `wip` ja esta marcada e uma sem veredito continua sem
  veredito) e lida: se cita uma funcao, tipo ou valor da mesma unidade (um que e
  de teste, ou que esta num `_test.go`, so conta para quem tambem e de teste), ou
  por `pacote.Nome` (o nome local de um import da pre-imagem, ver abaixo) o de outra, colocado depois da
  ativacao (o lugar da ancora anterior da chave, ou o commit onde ela nasce), o
  veredito e de reprovada: `fold`, ou `wip` sob essa politica, com a declaracao
  no motivo. Um metodo conta quando a pre-imagem seleciona `Nome` e o texto final
  da MESMA declaracao nao o tem, e o metodo nasce depois da ativacao, na mesma
  unidade ou numa que o arquivo importa. A pre-imagem seleciona `Nome` quando tem
  `.Nome` (um seletor cujo receptor nao e um nome solto: um local, outro seletor
  como `s.bbb.Nome`, uma chamada, um indice, um literal) ou `q.Nome` com `q` um
  nome solto que nao e o nome local de um import da pre-imagem; assim um
  qualificador de pacote (`entities.Plan`) nao e um seletor de metodo, e uma
  variavel de pacote solta e. Um local que tem o nome de um pacote importado
  vira `.Nome` e conta. O texto final tem `Nome` so quando tem `.Nome`: um
  `q.Nome` com `q` solto no texto final nao o tem, seja `q` um pacote
  (`hlp.Nome(t)`) ou uma variavel de pacote, o que dobra a ancora sem ler os
  imports do arquivo final. Uma variavel de pacote chamada nos dois textos vira
  assim um falso descarte, que e aceito. Quando os dois textos tem `.Nome` do
  mesmo referente, a declaracao final ja tem uma aresta ate o metodo e a ordem a
  coloca depois dele; so quando os dois nomeiam o mesmo referente: um `.Nome` de
  outro referente em cada texto nao e visto (limite abaixo), e um seletor que a
  correcao tirou pode nomear algo nascido depois. Sem tipos nao da para dizer
  de que tipo e o seletor: o metodo de outro tipo com o mesmo nome tambem conta,
  um campo com esse nome tambem conta, e um metodo mais antigo com esse nome nao
  salva um mais novo. Por isso o motivo pode nomear um metodo que a pre-imagem
  nao le de verdade (medido no alvo: a ancora 157d218 descarta com o motivo
  `reads ...assertionRule.Name` quando `d.Key.Name` e um campo); o falso descarte
  e aceito, e o motivo diz `may read`. Uma entrada que nao e declaracao (um
  `README.md`) nao le nada.
  Os nomes lidos vem de uma analise sintatica quando a porta `Reader` esta ligada
  e o arquivo e Go: sao os identificadores que o texto da pre-imagem nao resolve
  sozinho, a chave solta de um literal composto que nao e um local nem um
  parametro (sem tipos pode ser um campo ou uma leitura), `qualificador.Nome`
  quando o qualificador nao se resolve e `.Nome` para todo seletor cujo
  receptor nao e um nome solto; um local, um parametro, um comentario ou um
  literal de texto nao contam, e o `.Nome` de um seletor dividido em linhas
  conta. A analise de cada texto se faz uma vez por entrada. Quando a
  pre-imagem nao analisa sozinha (uma instrucao de funcao cortada, `Nome#i`, ou
  um arquivo que nao e Go) o seletor vale pela busca de texto `.Nome`, nos dois
  textos, com o qualificador de pacote fora. Se so o texto final nao analisa, ele
  nao tem nada e a ancora dobra. Para os outros nomes, sem a porta, ou quando o texto nao analisa
  sozinho (uma instrucao de funcao cortada, `Nome#i`),
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
  descartar uma ancora que compilaria. Os nomes locais de um pacote lido vem dos
  imports da pre-imagem cuja ultima parte do caminho e a base do diretorio da
  unidade (o apelido de cada um, ou a base quando nao tem), mais a propria base.
  So uma raiz de modulo (a raiz do repositorio, ou a de um modulo aninhado, que
  o grafo mostra pela aresta `module` ate o `go.mod` do diretorio) pode importar
  sob uma base diferente da do diretorio, e so ela conta todos os imports da
  pre-imagem, por excesso. Em qualquer outro diretorio a base do caminho do
  import tem de ser a do diretorio; se nenhum import a tem, nada daquele pacote
  e lido por qualificador. Limites: um metodo de um pacote que nenhum import
  nomeia pela base do diretorio nao e visto, nem a leitura por um import com
  ponto (`import . "x"`) ou por um pacote cujo nome difere do diretorio sem
  apelido (e esse nome tambem nao e tomado por pacote no seletor); um mesmo
  `.Nome` nos dois textos com outro referente em cada um nao e visto aqui
  (medido: `t.M()` virando `bbb.U{}.M()`, ou `t.X.M`, no texto final); e uma
  pre-imagem que entrega um tipo
  a uma interface cujo metodo nasce depois compila na sonda e quebra no commit
  (medido: a ancora fica `kept` e o `verify` falha na posicao 1 com `bbb.T does
  not implement bbb.I (missing method M)`). Tambem ficam `kept` e vermelhos, sem
  marcador, o metodo de um import com ponto (`import . "x"`) e o promovido por
  um tipo embutido de uma unidade que o arquivo nao importa. O `verify` do
  plano pega esses casos.
- Uma pre-imagem que nao pode ser costurada nem renderizada (um `go.mod` que a
  correcao transformou em bloco, uma casca com o mesmo trecho em duas entradas)
  nao pode ser emitida: o veredito e "inadequada" e a ancora cai em `fold` mesmo
  sob `wip`. Uma edicao fora da arvore e uma falha de leitura continuam sendo
  erro. Entradas e edicoes que dividem o mesmo trecho com o mesmo texto (um grupo
  `const (...)`) valem uma so; o mesmo trecho com textos diferentes
  (`var a = 1; var b = 2` virando `var a, b = 3, 4`) e inadequado para a sonda e
  para o emissor, que costuram com o mesmo codigo (`emitting.Compose`). O
  planejador nao deixa esse caso chegar la dentro de uma ancora: os nomes de
  pre-imagens diferentes saem dela com o aviso `anchors: not spliced` (ver
  abaixo), e a ancora fica com o que sobra. Entre ancoras o emissor costura toda
  pre-imagem escolhida, tambem as de uma ancora `wip` ou sem veredito, que a
  sonda de uma mais antiga nao leva: uma pre-imagem cujo trecho se sobrepoe ao
  de outro nome de uma ancora mais nova assim escolhida e inadequada e a ancora
  cai em `fold`, mesmo sob `wip`, com o motivo `its pre-image overlaps <chave>,
  which a newer anchor keeps`.
- A sonda constroi o que o emissor escreve: o arquivo final com as
  pre-imagens ativas e os imports delas, pela mesma `Compose`. Como o emissor
  escreve um commit so com as declaracoes ja nascidas, depois de decidir cada
  ancora o planejador sonda de novo as pre-imagens restritas as declaracoes
  nascidas antes do `slot` dela, da ancora mais nova para a mais antiga: a
  primeira que quebra nesse estado cai em `fold` (ou em `wip`) e as mais antigas
  sao julgadas de novo, ja sem ela. Um estado que o renderizador recusa nao e
  o veredito "inadequada" de cima: segue a politica, e sob `wip` marca a ancora.
  Limite: a sonda olha o estado logo antes do `slot`, nao cada commit
  intermediario da janela, e esse estado so tem o que nasceu antes: um metodo que
  nasce depois da pre-imagem que o chama e pego pela leitura acima, nao por ele.
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
- Uma pre-imagem leva os imports que o seu texto usa, lidos do arquivo PAI da
  ancora (`Version.Imports`, `Edit.Imports`), pela porta opcional `Importer` da
  linguagem (`Needed`); o emissor e a sonda os acrescentam ao arquivo final com a
  mesma `Admit` opcional, antes de renderizar, e o renderizador depois poda o que
  nada mais usa. Nunca vai `import "C"`, `_` ou `.`. Varias ancoras no mesmo arquivo
  valem a uniao. Uma declaracao movida toma os imports do arquivo de origem para o
  arquivo final. Um apelido que o arquivo final ja usa para outro pacote, ou um
  pacote que o `go.mod` nao tem, nao compila: a sonda diz isso e, com a politica
  `fold`, a ancora cai em `fold`, com o motivo; com `wip` o resultado e `wip` e os
  commits ficam marcados (r38). Sem `Importer` ou sem `Admit` na linguagem, nada se
  acrescenta.
- Nomes que dividem um trecho final (um grupo `const` com `iota`, `var a, b = ...`)
  e tem pre-imagens diferentes entre si, e um nome declarado duas vezes no mesmo
  arquivo (`init`), nao se costuram: saem da ancora com o aviso
  `anchors: not spliced` e o motivo, e a ancora sem mais nada e descartada. Um
  nome que divide o trecho e nao tem pre-imagem (nasceu na ancora) nao discorda
  de ninguem: o grupo que ganha um nome mantem a correcao dos outros (r16) so
  quando nada usa o nome novo; com um usuario dele a pre-imagem nao compila no
  commit da ancora e a ancora cai em `fold` com o motivo `undefined: C`. Um
  nome que divide o trecho com outro deixado de lado tambem fica de lado. Uma
  correcao que reescreve um grupo com pre-imagens diferentes por nome deixa
  esses nomes como estao e avisa; o resumo da ancora descartada diz "every
  placed declaration it changes is left alone" quando alguma declaracao
  colocada ficou de lado, e "it changes no placed declaration" quando nenhuma
  das que ela muda esta colocada (uma apagada depois, pulada com "absent from
  the final tree", so conta aqui).
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
- Limites conhecidos: o import de uma pre-imagem e achado pelo nome local que o
  renderizador adivinha pelo caminho; um pacote cujo nome difere da base do
  caminho e sem apelido pode nao ser achado, e o erro e um `fold` (ou `wip`);
  a sonda compila com o `Verify` da configuracao (`verify.exclude` e `tags`):
  uma unidade excluida nao e construida, nem quando uma unidade compilada a
  exige (compila so o que importa dela), e a ancora cujas edicoes so tocam
  unidades excluidas fica `unchecked` ("every module the edits touch is excluded
  by verify.exclude"); se a edicao toca tambem uma unidade compilada, a nota
  (`built <unidades>; excluded by verify.exclude and not built: <unidades>`)
  nomeia depois do `not built` so as unidades excluidas donas de um arquivo
  editado, e uma excluida que apenas depende da editada nao entra; num modulo aninhado, os arquivos dele pertencem a ele, nao ao modulo
  da raiz. Uma correcao fora de Go e de qualquer modulo fica
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

### A politica `stop` e o `continue`

`policy: stop` (global ou por ancora) faz uma ancora cuja pre-imagem nao compila
parar a execucao em vez de dobrar ou marcar. So `plan` e `emit` param (codigo 3);
`audit` planeja e registra a ancora no resumo (`outcome=stopped`) mas termina com
0, e `report` nao planeja. A parada lista TODOS os conflitos, nao o primeiro, e
nao escreve nada na saida padrao nem no cache do plano (uma parada e um plano que
falhou, e o plano que falhou nao e guardado nem refeito). O resumo registra a
ancora como `outcome=stopped`, tambem com `--from-cache`, nunca como `folded`.
`stop` exige `--state-dir DIR`: sem ele e erro de uso (codigo 2) antes de
qualquer indexacao, tambem para um `stop` so numa ancora, e nada e escrito num
lugar escondido. A chave do cache nao inclui `--state-dir`.

Codigos de saida: `0` terminou; `1` falhou; `2` uso ou configuracao invalida;
`3` parou numa ancora e o estado ficou guardado em `--state-dir`. Se o estado nao
pode ser guardado (`DIR` e um arquivo, o `conflicts/` ou um nome reservado de
`DIR` tem algo que nao e do redigit, o disco falhou), a execucao termina com `2`
(uso) ou `1`, nunca com `3`, e a mensagem diz que o estado nao foi guardado e que
repetir com `policy` `fold` ou `wip`, ou com outro `--state-dir`, segue. `DIR` pode
ter arquivos do usuario: a parada os deixa onde estao (um `notes.txt` ao lado
segue la, codigo 3); a recusa e sobre o `conflicts/` e os nomes reservados.

Em `DIR` ficam `state.json` (com a marca `redigit-stop-state`: a linha de comando
original, o commit resolvido, o digest da configuracao original, as decisoes ate
ali, se a parada criou `DIR`, e os conflitos; e a ultima coisa gravada, entao uma
parada interrompida, ou que falha, deixa o estado anterior valido, e sem ele nao
ha estado),
`conflicts/<rev>/NN-<nome>.preimage` e `.final` (os dois textos de cada
declaracao; um nome com mais de 120 bytes vira um prefixo e um hash) com o
arquivo `.redigit-conflicts` em `conflicts/`, e, depois de um `continue`,
`.redigit-continue.json`, a configuracao derivada. O `.redigit-conflicts` lista,
um por linha, cada texto que o redigit gravou, e e gravado antes deles; o redigit
so grava e so remove o que esta nessa lista e os nomes reservados e exatos
`.redigit-stage`, `.redigit-old`, `.redigit-continue.json` e
`.redigit-tmp-<digitos>`. Um texto listado que voce editou vai junto com o resto.
Um arquivo comum num desses nomes reservados exatos conta como do redigit e sai.
Uma pasta nesses nomes so sai vazia ou com o que a marca lista; com outra coisa
dentro ela fica, com a marca ao lado: o `emit` recusa com codigo 2 e a nomeia, e o
`--abort` termina com 0 e a deixa, sem nomea-la. Um `state.json` ou um `conflicts/` sem marca
em `DIR` e recusado (codigo 2, o arquivo e nomeado) e nada e tocado. Um
`continue` com um `conflicts/` marcado que tem uma entrada fora da lista (um
arquivo ou uma pasta do usuario) roda; se ele para de novo, o `Save` recusa em
vez de apagar (codigo 2, a entrada e nomeada, o estado fica), e se ele termina
devolve 0 e deixa a entrada. A unica excecao a recusa de um `conflicts/` sem marca
e um `conflicts/` vazio e sem marca ao lado de um `state.json` do redigit, que uma
parada interrompida deixa e que e reaproveitado. Um `conflicts/` que e link
simbolico nunca e tocado. `--config` nao pode nomear `.redigit-continue.json`.
Duas execucoes ao mesmo tempo sobre o mesmo `DIR` nao sao suportadas.

`redigit continue --state DIR [--fold REV] [--wip REV] [--rest fold|wip] [--abort]`
segue dali. Cada decisao nomeia um conflito por um prefixo nao ambiguo do rev;
uma ancora sem decisao para de novo (codigo 3), e o `continue` guarda o estado de
novo, sobre o original, com todas as decisoes ate ali. `--rest fold|wip` decide
todas as que ainda param, e o resultado de um `continue` que termina e identico,
byte a byte, ao de uma execucao nova com as mesmas decisoes (`policy: fold` ou
`wip` para todas, ou a decisao de cada ancora). O `continue` le a linha de
comando guardada com a gramatica de flags do Go e a reexecuta com a
configuracao original mais as decisoes, so na chave `lineage`, em
`.redigit-continue.json`. Ele recusa, sem executar nada, quando: o commit ou a
configuracao original mudaram desde a parada (codigo 1); a configuracao derivada
passaria de 1 MiB (codigo 1); `DIR` tem um `state.json` ou um `conflicts/` sem a
marca do redigit, ou nao tem estado (codigo 2); ou o estado guarda uma linha de
comando que e um `continue` (codigo 1). Uma execucao que falha (codigo 1) deixa
o estado como estava, e o mesmo comando segue; ela pode deixar parte de
`conflicts/` ja trocada e um `.redigit-old`, o estado fica intacto, e corrigir a
causa e repetir o mesmo comando, ou `--abort`, segue. Essa lista e a das recusas
conhecidas, nao uma garantia de que nenhum outro caso existe. `--abort` remove
so os textos da lista da marca (e as pastas `<rev>` que ficam vazias),
`.redigit-continue.json` e as sobras reservadas do redigit, e por ultimo o
`state.json` com a marca, de modo que uma remocao interrompida pode ser repetida;
um primeiro save interrompido e depois uma remocao interrompida podem deixar um
`conflicts/` vazio e sem marca, sem `state.json`: `emit` o recusa nomeando-o e
`continue` e `--abort` dizem que nao ha estado (codigo 2), e remover o
`conflicts/` vazio, ou usar outro `--state-dir`, segue; o `DIR` so sai se a parada
o criou e ele ficou vazio, e o que o usuario guardou la dentro fica e faz o `rmdir` falhar, de proposito. Se uma troca de `conflicts/`
foi interrompida, o `state.json` e as decisoes ficam e o `conflicts/` pode ja ser
o novo. Um `conflicts/` marcado sem `state.json` e uma parada interrompida:
`continue` diz que nao ha estado (codigo 2) e `--abort` o limpa (codigo 0), assim
como um `state.json` com a marca mas sem linha de comando. Um
`--edited REV` (uma pre-imagem editada a mao) nao existe nesta fatia: o formato
de entrada dele ainda nao esta definido, e `continue` o recusa como flag
desconhecida (codigo 2). `write` e `verify` leem um manifesto e nao planejam,
entao nao param.

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
