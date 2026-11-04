# redigit - plano de arquitetura

Ferramenta que reconstroi a historia de um repositorio de modo que cada commit
crie exatamente o que precisa: nada nasce antes de quem o usa, o teste nasce com
a funcao que exercita, e os commits tem a forma que uma pessoa escreveria.

A unidade e a DECLARACAO, nunca o arquivo. Um arquivo nasce com tres funcoes e
ganha oito ao longo de meses; so uma declaracao pode carregar posicao propria.

## 1. O que a pesquisa estabeleceu

Seis frentes de investigacao, com o que cada uma decidiu.

### 1.1 A lacuna e real

Duas linhagens de trabalho anterior, nenhuma faz isto:

- Untangling academico (ClusterChanges/Microsoft, Flexeme, UTango, SmartCommit,
  Atomizer 2026) DIVIDE um diff existente em grupos. Nunca propoe ordem de
  construcao nem sintetiza estados intermediarios.
- Ferramentas de historia (git-absorb, hg absorb, jj absorb, git-deps,
  git-explode, git-branchless, filter-repo) REORDENAM ou anexam commits que ja
  fazem sentido isoladamente.

O primo mais proximo e o CSlicer (ASE 2015), que fatia historia por teste, mas
extrai fatia existente em vez de inventar commits, e opera em granularidade de
commit, nao de declaracao.

Ideias que valem roubar:

- def-use como sinal primario (ClusterChanges relatou zero falsos positivos)
- o delta-PDG multi-versao do Flexeme, que e o grafo que precisamos
- a recusa explicita do `jj absorb` em agir quando a atribuicao e ambigua
- o teste como unidade de intencao (CSlicer)

Armadilha medida que nos atinge diretamente: o AtomicCommitBench (2026) mostra
agentes com replay quase perfeito (PPAR > 0.988) e qualidade de agrupamento
pessima (ARI 0.03 a 0.46). **"A arvore fecha" e "a historia faz sentido" sao
duas metricas.** O invariante da arvore final nao diz nada sobre a segunda.

### 1.2 Os dois motores de resolucao, medidos

| | LSP ao vivo (gopls v0.23) | SCIP (scip-go) |
|---|---|---|
| `references` | ~10 ms quente -> ~100 s para 10k decls | incluso no indice |
| modulo inteiro | n/a | 0,67 s (59 documentos, 419 implementacoes) |
| `implementation` | funciona; custo em escala nao medido aqui | verificado: 54 relacoes cruzando modulo |
| build tags | `{"buildFlags":["-tags=integration"]}` (verificado) | `GOFLAGS=-tags=integration` (verificado) |
| multi-modulo | `go.work` funciona (testado com 2 modulos) | NAO suporta `go.work`; roda por modulo |
| fronteira de declaracao | `documentSymbol` da `range` | `EnclosingRange` so em 119 de 986 |
| papel Test / Generated | nao existe no protocolo | scip-go nao popula (ambos zero) |
| por linguagem | qualquer servidor LSP | precisa de indexador, ou ponte lsp-to-scip |

Os dois funcionam. Nao e bifurcacao, e um seam com dois adaptadores reais.
Comeca pelo LSP.

Armadilhas verificadas que teriam custado caro:

- gopls **nunca anuncia `positionEncoding`** e opera sempre em UTF-16. A
  conversao de offset e nossa.
- As chaves de `initializationOptions` sao achatadas no ultimo ponto:
  `build.buildFlags` == `buildFlags`. JSON aninhado nao funciona.
- Metodos NAO vem aninhados sob o tipo receptor no `documentSymbol`.
- `didOpen` nao e necessario para `documentSymbol`/`references`.
- Um arquivo com build tag aberto por `didOpen` responde `documentSymbol` mesmo
  sem a tag, mas fica invisivel a `references`/`workspace/symbol`. E o mecanismo
  exato dos 194 arquivos invisiveis: a consulta sintatica mente que esta vendo.

### 1.3 Escrita da historia

`git fast-import` por larga margem. Medido em 300 commits x 200 arquivos
(60 mil blobs):

| mecanismo | tempo | `.git` resultante |
|---|---|---|
| `fast-import` | 4,7 s | 6,1 MB, um pack |
| escritor Go de objetos soltos + `write-tree` | 10,9 s | 235 MB, 60k objetos soltos |
| laco ingenuo `hash-object`+`update-index` | ~365 s (extrapolado) | - |

Determinismo confirmado: o mesmo fluxo em dois repositorios limpos produziu ref
bit-identica. Datas e identidade vao dentro do fluxo, nao em variaveis de
ambiente.

### 1.4 Mecanismo de plugin

Registro em tempo de compilacao, como `go/analysis` e o golangci-lint. Nao
`plugin`, nao subprocesso, nao WASM. Um binario estatico, CGO desligado.

Tree-sitter foi avaliado e **recusado por ora**: o binding oficial exige CGO, e
o unico runtime "puro Go" disponivel (`gotreesitter`) tem 523 arquivos num
pacote plano, 330 deles de teste, funcoes `...ForTest` exportadas na API publica
e um `func Add(a, b int) int` no topo. Para Go usamos `go/ast` mais
`github.com/dave/dst`, que existe precisamente para resolver o problema de
comentarios que se soltam do no.

## 2. Arquitetura

Seis estagios. A superficie especifica de linguagem e menor do que parece.

```
  historia de origem (opcional) ----+
                                     |
  arvore final --------------------- v
                              [ Snapshot ]
                                     |
                    +----------------+----------------+
                    v                                 v
             [ Resolver ]                       [ LanguagePack ]
        arestas semanticas exatas          declaracoes, spans, render,
        (LSP ou SCIP; agnostico)           arestas invisiveis (por linguagem)
                    |                                 |
                    +----------------+----------------+
                                     v
                                 [ Graph ]
                        declaracoes + arestas + motivos
                                     |
                                     v
                              [ Orderer ]
                  ponto fixo; ciclos NOMEADOS, nao escondidos
                                     |
                                     v
                              [ Planner ]
                 clusters de feature; kernel compartilhado por camada
                                     |
                                     v
                          [ Plan ]  <-- REVISAO HUMANA/IA
                          arquivo legivel e editavel
                                     |
                                     v
                              [ Writer ]
                     render por commit; git fast-import
                                     |
                                     v
                    nova historia + [ Auditor ]
```

### 2.1 As tres portas

Cada uma tem dois adaptadores reais ou um plano concreto para o segundo. Uma
porta com um adaptador so seria seam hipotetico, e a skill de design proibe.

**`Resolver`** - fatos semanticos, agnostico de linguagem.
Adaptadores: gopls via LSP, e SCIP. Responde quem referencia o que, o que
implementa o que, onde algo e declarado.

**`Syntax`** - fronteiras de declaracao e renderizacao parcial, por linguagem.
Adaptadores: Go (`go/ast` + `dst`), e um recuo generico que usa
`documentSymbol` para as fronteiras e corte por linha para o render.

**`RulePack`** - as arestas que nenhum grafo de referencia enxerga, por
linguagem. Para Go: goverter, mockgen nos tres formatos de cabecalho, sqlc
`-- name:`, a migracao entre o schema e a query, e a visibilidade por build tag.

### 2.2 Por que o render fica com a linguagem

`go/ast.FilterFile` documenta que nao mexe em `File.Comments`. Filtrar
declaracoes de forma ingenua deixa comentarios orfaos em posicao velha, ou come
a linha `//go:build` acima da primeira declaracao removida. O corpo de trabalho
anterior encontrou esse bug em producao e o corrigiu ancorando o span no
`Doc.Pos()` e nao no `decl.Pos()`. O `dst` resolve na raiz. Nenhum formato
agnostico de indice substitui isto.

### 2.3 O carregador tipado roda com -trimpath

Cada execucao materializa a arvore num diretorio novo em `/tmp`. Sem
`-trimpath` o cache de build do Go e chaveado por esse diretorio: toda execucao
recompila tudo e deixa dezenas de MB no `GOCACHE`. O carregador de
`langpack/golang/typed` passa `-trimpath` e le o modulo (`NeedModule`) e os
arquivos que o go listou (`NeedFiles`) de cada pacote. Com isso o export data
nomeia arquivos pelo caminho do modulo (`mod/pkg/f.go`, ou `mod@versao/pkg/f.go`
para todo modulo com versao) e nao pelo disco. `typed/modmap` volta desse nome
ao arquivo da arvore: o dono e o modulo de caminho mais longo, achado em todo o
grafo de imports (`packages.Visit`), nao so no primeiro nivel. O diretorio de um
modulo sem `Dir`, o vendor, nao e adivinhado: vem do arquivo que o go listou
para o pacote. Assim vale o vendor ao lado do `go.work`, com ou sem modulo na
raiz, e o vendor velho de um membro nunca e lido. Um modulo que dois pacotes
colocam em diretorios diferentes, e um pacote do qual o go nao listou arquivo,
leem como fora da arvore, igual a um nome que nao se resolve (biblioteca padrao,
cache de modulos, replace fora da arvore): igual ao `outside` de
`resolve/uses.go`, nunca como outro arquivo.

Um pacote com cgo guarda no cache, na `//line` do arquivo gerado, o checkout que
encheu o cache, e o grafo passava a depender de qual checkout foi esse.
`modmap` troca esse nome pelo arquivo de mesmo nome base que o go listou nesta
arvore, antes de olhar se o nome e absoluto, e so para um arquivo que o go nao
listou: um arquivo listado foi lido onde esta, e a sua `//line` nomeia o que o
autor escreveu. O `Offset` de uma declaracao num arquivo cgo conta no arquivo
listado e nao no gerado, cujo texto da `//line` leva o tamanho do checkout que
encheu o cache. Os fixtures com cgo ganham arestas verdadeiras e perdem so as
falsas em relacao a base, cuja contagem muda com o tamanho do checkout. Nove tamanhos de
`TMPDIR`, um cache novo por tamanho, dao o mesmo grafo normalizado do tip, e um
cache enchido de um `TMPDIR` longo e lido de `/tmp` tambem. T e amigonimo nao
tem esse formato, entao so os fixtures provam.

Uma diretiva `//line` que sai do diretorio do pacote (`//line ../other/o.go:3`)
deixa no export data o caminho absoluto do checkout que encheu o cache.
`ObjectAt` recusa um nome absoluto que o proprio pacote do objeto nao lista.
Mudanca visivel ao dono: as duas arestas que a base dava para o arquivo nomeado
pela diretiva eram falsas e somem; num fixture com esse formato a base deu 86
arestas e o tip 84, nas tres execucoes de cada jeito de `GOFLAGS` sobre um
cache novo. A contagem de recuos do resolvedor (`typed resolver fallbacks`) num
`//line` que escapa pode variar entre execucoes; as arestas saem iguais.

Medido em `157d218` (T), comando `graph`, `GOCACHE` novo e aquecido por uma
execucao, tres execucoes cada, com `GOFLAGS=-p=2` e com `GOFLAGS="-trimpath
-p=2"`; a faixa de cada linha junta tres leituras independentes. Com tags: base
2,78 a 3,20 s nos dois jeitos, tip `-p=2` 1,03 a 1,19 s, tip `-trimpath -p=2`
0,96 a 1,18 s. Sem tags: base 4,67 a 5,24 s nos dois jeitos, tip `-p=2` 2,25 a
2,57 s, tip `-trimpath -p=2` 2,11 a 2,91 s. Crescimento do
`GOCACHE` na segunda execucao de outro diretorio, T com tags: base 29772 KB, tip
24 KB nos dois jeitos; T sem tags: base 38192 KB (`-p=2`), tip 224 KB (`-p=2`)
e 24 KB (`-trimpath -p=2`). Amigonimo (clone de `2502d84`): com tags base
72440 KB, tip
1180 KB e 540 KB; sem tags base 72408 KB, tip 1132 KB e 540 KB. As arestas de T
(20919 com tags, 21585 sem) e as de amigonimo (19757 com tags, 19534 sem) saem
iguais as da base nos dois jeitos de `GOFLAGS`; a comparacao foi feita sobre o
grafo normalizado, nao sobre os bytes da saida.

Um valor que escapa para uma interface como ponteiro de ponteiro (`var alvo
*MeuErro; errors.As(err, &alvo)`, `json.Unmarshal(dados, &ponteiro)`) tem
conjunto de metodos vazio. `escape` le os metodos do que ele aponta, e o dono
do tipo tira todos os ponteiros: o leitor ganha aresta `implements` para
`Error`, e `go vet` deixa de ver o metodo nascer depois do leitor. Em T
(`157d218`, com tags) sao 14 arestas a mais, todas de `errors.As`, nenhuma a
menos, nenhuma declaracao alterada.

Limite: um metodo do proprio tipo com alvo por valor (`func (E) Wrap(err error)
bool { var alvo E; return errors.As(err, &alvo) }`) segue sem aresta para `Error`,
pois `escape` so ve o tipo `*E` e nao separa esse alvo do receptor que passa por
interface (`fmt.Errorf("%v", receptor)` dentro de `Error`), que nao pode ter aresta.

## 3. A decisao vertical contra horizontal

Ordenar por dependencia produz camadas horizontais. Quem escreve a mao produz
vertical. Fundir vizinhos nao converte, porque o par vertical fica a centenas de
commits de distancia. Teto medido pelo trabalho anterior: ~73% num diretorio so,
contra alvo de 46-52%, com quatro auditorias independentes convergindo nele.

**A regra, derivada do grafo e nao de um botao:**

Para cada declaracao, contar em quantos clusters de feature distintos vivem os
seus consumidores.

- consumidores num cluster so -> nasce COM aquela feature (vertical)
- consumidores atravessando muitos clusters -> e nucleo compartilhado e nasce
  na feature que PRIMEIRO precisa dele, nos commits logo antes dos dela. Um
  bloco de camada antes de todas as features e uma centena de commits que
  nenhuma feature explica, lidos pela primeira vez centenas de commits depois.

E a mesma forma da regra 7 do documento de aprendizados (um pacote nasce quando
alguem DE FORA precisa dele), contando features em vez de pacotes.

Casos onde camada e a resposta honesta, e nao uma derrota:

1. **Bootstrap do repositorio.** go.mod, config, logging, tipos de erro. Nao ha
   feature a que pertencam.
2. **Blocos gerados.** `dbgen` com 409 metodos sai de uma execucao do gerador.
   Espalhar isso por 40 commits de feature e menos verdadeiro, nao mais.
3. **Kernel compartilhado.** `Money`, `ID`, `Clock`. Atribuir a qualquer feature
   e mentira.
4. **Biblioteca pura.** Medido: `goremy-di` tem 5% de commits multicamada,
   contra 32-33% em `jane` e `stark`. Uma biblioteca nao tem camadas para
   atravessar, e perseguir o alvo de aplicacao nela e perseguir o numero errado.

**A regua de forma nao e um numero global.** Ela depende do tipo de
repositorio, e a ferramenta reporta a sua, nunca persegue a de outro.

### 3.1 O plano e sempre revisado

Nenhuma historia e escrita direto de agrupamento automatico. O `Planner` emite
um arquivo de plano legivel; uma pessoa ou a IA que opera a ferramenta ajusta;
so entao o `Writer` escreve.

Isto nao e cautela vaga, e o que a evidencia manda: o AtomicCommitBench mediu
ARI de 0,03 a 0,46 para agrupamento automatico, e o ChangeBeadsThreader existe
precisamente porque agrupamento 100% automatico se mostrou inusavel na pratica.
O `jj absorb` recusa-se a agir quando a atribuicao e ambigua, e essa recusa e
uma feature.

### 3.2 O corte do nucleo compartilhado fecha sobre si mesmo

Medido num repositorio real de 505 arquivos: `Event` nascia no commit 25 de
247, mas os tipos dos seus proprios campos (`AckFunc`, `Kind`, `QRPayload`,
`InboundMessage`, `ReceiptBatch`, `reason.Reason`) so nasciam depois, o ultimo
na posicao 203. Uma duzia de outras declaracoes repetia o padrao, com lacunas
de 180 a 200 posicoes - `emit` reportava 850 riscos de referencia para frente.

A causa nao estava no ponto fixo: `order.Of()` ja calculava a posicao certa
para `Event`, tarde o bastante para vir depois de tudo que ele precisa. A
causa era o corte puxar o nucleo compartilhado em UM SO SALTO. Quando a
primeira feature que le `Event` pede o que ainda nao nasceu, o corte incluia
`Event` porque `Event` mesmo era um pedido direto - mas nao olhava o que
`Event` por sua vez precisa, entao `reason.Reason` ficava para tras, a espera
de outra feature que o pedisse por conta propria.

O corte agora fecha sobre o proprio nucleo: puxar uma declaracao compartilhada
tambem puxa, transitivamente, toda declaracao compartilhada QUE ELA MESMA
precisa e ainda nao nasceu. Isto nunca alcanca a declaracao de uma feature
ainda nao processada - a mesma prova de ownership que impede duas features de
precisarem uma da outra (secao 2, `assign`) garante que tudo que uma
declaracao compartilhada precisa e alcancado pelas MESMAS features que a
alcancam, logo tambem e compartilhado. O fecho so cresce o commit que ja
estava sendo cortado; nunca espera por uma feature adiante na fila. Dentro do
commit, a ordem de nascimento de cada membro continua vindo de `order.Of()`,
que e quem sabia a resposta certa o tempo todo.

### 3.3 Quatro defeitos no corte, medidos numa reconstrucao real de 195 commits

Uma auditoria independente da historia reconstruida (mesmo repositorio de 505
arquivos) achou quatro defeitos na politica de agrupamento e corte, nenhum no
ponto fixo.

**Teste separado do codigo.** `entryPoints` (secao 2) exclui um teste de ser
ele mesmo um ponto de entrada, e com razao - mas nada o atribuia a feature do
que ele exercita. Sem dono, um teste caia sempre no balde compartilhado e so
nascia no corte final (`remaining`), longe do sujeito que testa: 205 pares
`foo.go`/`foo_test.go` com mediana de 90 commits de distancia, so 3 no mesmo
commit. A correcao e em duas camadas: `inheritTestOwnership` copia o dono do
alvo que a `Anchor` de `test-of` ja aponta (a mesma ancora que a secao 1.3 do
CONTRATO-EMISSAO usa para aproximar o `order.Of()`); `inheritTestFileOwnership`
cobre o teste que a ancora por nome nao resolveu, herdando o dono do arquivo
irmao (`foo_test.go` de `foo.go`) quando esse arquivo tem um dono unico. A
segunda camada so se aplica quando as PROPRIAS necessidades do teste nao
cruzam para outra feature - herdar cegamente criou um ciclo entre dez
features na primeira tentativa, porque um teste que le um helper de outra
feature vira, sem essa guarda, uma aresta real entre as duas.

**Prosa como ponto de entrada.** Um arquivo `KindOther` de fallback (`docs/*.md`,
`LICENSE`) que nada le e nada consome vira, pela regra da secao 2, um ponto de
entrada de uma feature so - o proprio arquivo. `docs/**` nascia na posicao
12-15 de 195, e o PLANO-EMISSAO diz que documentacao geral pertence ao fim.
A causa e que TODO arquivo carrega uma aresta `module` (secao 8) so por estar
dentro do modulo, entao "sem aresta nenhuma" nunca e verdade. `isOrphanProse`
ignora essa aresta estrutural (`module`, `hosts`, `workspace`) ao decidir se
um `KindOther` e um ponto de entrada de verdade; sem needs alem dela e sem
consumidor, cai no balde compartilhado e nasce no corte final, como um
`deps.go` de import em branco (que tem um need real) continua sem cair.

**Atomo dividido entre dois pedidos.** Um arquivo gerado (`Atom`, secao 5)
nasce inteiro quando o primeiro pedaco e pedido - mas o corte so olhava as
arestas de QUEM pede, nunca os outros membros do mesmo atomo. `WwebjsAuthSession`
foi pedido no commit 91 por uma feature tardia; as outras seis declaracoes do
mesmo `models.go` ja tinham sido renderizadas no commit 38 por outra feature,
e o renderizador (que sempre emite o arquivo por inteiro) ja incluia
`WwebjsAuthSession` ali. O commit 91 virava um commit vazio - a arvore nao
mudava, mas a feature achava que tinha nascido algo. A correcao fecha o
`unbornProviders` (secao 3.2) tambem sobre o atomo: puxar uma declaracao
compartilhada agora puxa toda irma de atomo ainda nao nascida junto, e
`assign` uniformiza o dono do atomo pela UNIAO de quem alcanca qualquer um dos
seus membros, para as duas metades nunca terminarem em features diferentes.

**Corte mecanico no limite de arquivos.** `cut` fechava um commit assim que
`MaxFiles` estourava, sem olhar se aquele arquivo era o meio de um pacote
coerente: 73 dos 199 commits medidos tinham exatamente 12 arquivos, o teto
configurado, e a auditoria achou commits tocando de 9 a 11 pacotes sem
historia em comum. `cut` agora lembra a ultima fronteira de pacote dentro do
commit corrente; ao estourar o teto, recua ate ali em vez de cortar no
arquivo exato que excedeu, e so cai no corte mecanico quando um pacote
sozinho e maior que `MaxFiles` (nao ha fronteira anterior para recuar). O
teto continua sendo um limite, nunca a regra que decide onde cortar.

### 3.4 O ponto de composicao nasce por ultimo, e nao ha correcao para isso

`cmd/wasabizap/main.go` e `bootstrap.Run` nascem na posicao 167 de 195 (85% da
historia), enquanto a historia humana original os introduziu no commit 2.
Medido diretamente no grafo: `main()` tem `order.Of() = 227`, o MESMO valor
maximo de toda a arvore de 3699 declaracoes - nenhuma outra declaracao chega
mais tarde. Isso nao e um defeito de corte: `main()` referencia a raiz de cada
subsistema, e o ponto fixo (secao 5 do documento, "toda medida declara o que
nao olha") so pode dar a ele a posicao de quem ele precisa por ultimo. A
historia humana original edita `main.go` incrementalmente, acrescentando uma
linha a cada modulo que fica pronto; este `Writer` renderiza o conteudo FINAL
de cada declaracao a cada commit (CONTRATO-EMISSAO secao 4), nunca um diff, e
"todo commit compila" exige que tudo que `main()` referencia ja exista. As
duas restricoes juntas fecham a porta: nao ha como nascer mais cedo sem
quebrar uma das duas.

### 3.5 A ancora `test-of` so cobria 175 dos 742 testes com sujeito

A secao 3.3 deu ao teste o dono do seu sujeito, mas so quando uma `Anchor`
`test-of` existia. A regra do pack Go so casava o nome inteiro do teste contra
um tipo (nunca uma funcao ou um valor) e nunca contra menos que o SUFIXO
inteiro na busca por metodo, entao `TestNormalizeProviderMessageID` - uma
funcao pura, sem tipo - nao gerava ancora nenhuma: o teste caia no balde
compartilhado e nascia 174 commits depois de `NormalizeProviderMessageID`.

Tres reforcos, nesta ordem de confianca:

- **corpo antes do nome**: se o corpo do teste chama exatamente UMA declaracao
  do pacote (identificador nu, nunca qualificado), essa chamada e um FATO e
  vence qualquer palpite do nome;
- **casamento por nome, mais longo primeiro**: alem da busca original
  (tipo cresce a esquerda, metodo encolhe a direita, o ultimo casamento
  vence), um recuo tenta o prefixo inteiro como tipo, funcao OU valor, depois
  cada prefixo mais curto - "TestSupervisorRunDrains" perde "Drains" e acha
  "Supervisor" mesmo sem achar o metodo "Run" exato;
- **arquivo irmao mais amplo**: quando o escopo do arquivo irmao (`foo.go` de
  `foo_test.go`) nao acha nada, a busca repete no pacote inteiro antes de
  desistir.

O resultado real: de 175 testes com sujeito identificado para 742, com 719
(96%) nascendo na mesma posicao do `order.Of()` do sujeito - medido por
`redigit audit`.

**Ownership sozinho nao bastava.** Um sujeito compartilhado (`SharedFeature`)
nunca e "precisado" por ninguem - nada tem aresta ATE um teste - entao
`unbornProviders` (secao 3.2) nunca o puxava, e o teste caia no `remaining()`
do fim mesmo com o dono certo. `unbornProviders` agora tambem puxa, junto de
cada declaracao compartilhada, qualquer teste ancorado a ela cujas PROPRIAS
necessidades ja estejam satisfeitas (nascidas, ou tambem compartilhadas e
alcancadas pelo mesmo fecho) - a mesma guarda de seguranca que protege atomos.

**Dois jeitos de duplicar uma declaracao, achados rodando no repositorio
real, nao no sintetico.** Primeiro: sem uma guarda equivalente a `needsOnly`
(secao 3.3) em `inheritTestOwnership`, herdar o dono de uma ancora para uma
feature real podia criar o mesmo ciclo entre features que a secao 3.3 ja
tinha corrigido para o recuo por arquivo irmao - a correcao e a mesma: heranca
de uma feature real so vale quando as proprias necessidades do teste
concordam; heranca de `SharedFeature` vale sempre, porque `SharedFeature`
nunca entra em `featureNeeds`. Segundo: um teste ja resolvido por uma ancora
podia ser re-adivinhado pelo recuo de arquivo irmao (secao 3.3), que via
"ainda compartilhado" e tentava de novo a partir do dono do arquivo irmao -
achando as vezes uma feature real e colocando o teste em DOIS commits (o da
feature, e o puxado pelo sujeito compartilhado). O recuo por arquivo irmao
agora ignora todo teste que uma ancora ja resolveu, sujeito compartilhado ou
nao.

### 3.6 A raiz de composicao nao e consumidora, e testes esperam o que exercitam

Num monolito com injecao de dependencia a raiz (`main`, o injetor, a tabela de
rotas) consome todo handler, sink e worker; nada sobrava sem consumidor alem
da propria raiz, e o programa inteiro virava uma feature compartilhada (47% da
historia, uma corrida de 323 commits `chore(shared)`). `findWiring` acha essas
declaracoes sem nomea-las: uma declaracao de producao cujo fecho cobre 1/20 do
programa (e ao menos `wiringFloor` declaracoes). Arestas dela nao contam como
consumo, entao os handlers viram pontas, e a raiz nasce depois da ultima
feature que ela registra.

Testes e tudo que mora em `_test.go` (helpers, mocks) sao adiados juntos com a
raiz: nascem logo depois da feature que completa o que precisam, nunca por
heranca de dono (a heranca por ancora e por arquivo irmao foi removida, porque
colocava o teste na feature que por acaso vinha primeiro). Um modulo lido de
fora que nao tem ponta ligada na raiz e uma biblioteca: nasce inteira com o
primeiro consumidor. As features saem em ordem gulosa: continuidade do modulo,
depois custo (fecho), depois posicao mais antiga.

Limite conhecido: o corpo unico de `DoInjections`/`run` registra tudo de uma
vez, entao a raiz nao cresce por declaracao.

### 3.7 Um produto nasce junto, e a ordem de modulos e uma preferencia

Agrupar features pelo modulo de build espalhava cada produto: rotas,
repositorio e worker de um mesmo contexto sao construidos em modulos
diferentes, e o arco de um contexto cobria a historia inteira. O produto e o
modulo-raiz mais as features ligadas a ele por contrato: uma declaracao lida por
poucas features (ate `kinReach`) liga todas elas com peso `1/(leitores-1)`, e a
feature entra na raiz a que esta mais ligada. Um produto sai de ponta a ponta:
a raiz, depois as camadas, e so entao o proximo produto; quando uma camada nao
pode sair, a proxima pronta do mesmo produto vai antes.

Uma feature tambem espera as raizes dos modulos que ela le, mas so como
preferencia: modulos que se leem mutuamente nao tem o que esperar, e o plano
sai mesmo assim.

O `go.mod` de uma biblioteca nasce com a biblioteca: a linha `require` de quem
a consome puxa o manifesto cedo, e o corpo so chegava com o primeiro codigo
lido, centenas de commits depois.

Um teste nasce no commit do que exercita (ou no seguinte): a feature que
completa o sujeito leva os testes prontos, e dentro do commit cada teste fica
logo depois do ultimo que ele precisa, sem partir um atomo.

### 3.8 Rotas: o registro nasce com quem o chama

A regra `go-router` (config `{handle, mount, groupMethods, paths?}`) entra no
mesmo pacote de idiomas da `go-di`, sem linguagem nova. Cada chamada que casa um
padrao de `handle` e uma rota: metodo, caminho (que pode ser cego, sem constante
que o de), handler (que pode ser anonimo) e a instrucao que a registra. Um
padrao casa o nome curto da chamada (`Router.GET`) ou o qualificado
(`github.com/go-fuego/fuego.Get`) e `*` nao atravessa `/`. Uma funcao de
montagem (alvo de uma chamada que casa `mount` cujo corpo registra rotas) nasce
junto com a primeira instrucao que a chama, desde que nem essa instrucao, nem o
atomo que o fatiador lhe deu, nem o grupo de recurso dela hospedem outra
montagem da mesma regra; as instrucoes de um mesmo recurso (mesmo caminho,
prefixo incluido, mesma funcao) podem nascer juntas com `groupMethods`. Um
rotulo de atomo vem da menor chave do
grupo, entao as duas passagens do indexador concordam; cada regra depois da
primeira tem o seu rotulo, e uma declaracao em duas regras fica no atomo da
ultima.

A visao `Routes(decls, facts)` entrega os fatos de rota aos cortes seguintes (e2e
e changelog). `Site` pode ser a funcao inteira quando ela nao foi cortada, e
`Path` vem unido ao prefixo do grupo. O portao conta montagens longe de quem as
chama (por posicao do ordenamento, nao por commit do plano: os dois podem cair
em commits diferentes), rotas repetidas dentro de uma funcao e handlers que
nascem depois da rota; so entra no total a rota com caminho constante e handler
nomeado. O texto de `Excluded` diz o que ele nao ve: rota por variavel de funcao, grupo aberto
por atribuicao, prefixo de grupo aninhado, caminho nao constante, constante que
nao e caminho, handler anonimo, montagem passada por variavel, rota de modulo
aninhado e padrao de `handle` que nao casa chamada alguma.

### 3.9 Pin: a espera declarada a mao

A regra `pin` (config `{decl, after}`, cada alvo `arquivo:tipo:nome` com globs por parte) acrescenta uma aresta dura `pin` de `decl` para `after`. O que o usuario declara vale ou falha alto:

- Um alvo que nao casa declaracao alguma falha ao montar o grafo (`graph`, rc=1); uma parte de arquivo que nao casa arquivo algum da arvore falha antes, em `bootstrap start` (rc=2).
- Um alvo cuja parte de tipo e literalmente `stmt` e recusado ao carregar a regra, dos dois lados: um pin nomeia a declaracao inteira e as instrucoes cortadas ainda nao existem quando as regras rodam pela primeira vez, entao se nomeia a funcao. Um glob de tipo so vale se tambem casar algo inteiro: um alvo que so casa instrucoes (`main.go:*:main#1`) morre na primeira passada, com a mensagem `pin matches no declaration before the cut`, e `main.go:*:*` passa porque casa tambem a cabeca.
- Uma declaracao de producao depois de uma de teste e recusada, nomeando as duas chaves: o planejamento de testes descarta a espera de producao por teste, e o pin se perderia em silencio. Teste depois de producao e teste depois de teste seguem permitidos.
- Um pin depois de uma funcao que foi cortada em instrucoes espera a funcao inteira: a assinatura e cada instrucao `Funcao#N` do mesmo arquivo, vistas na segunda passada das regras. Uma instrucao da propria funcao de quem espera nao cria aresta dela mesma.

Um pin que fecha um ciclo entre arquivos nao e desfeito nem recusado: `graph`, `plan`, `audit` e `emit` seguem adiante com um WARN ("the ordering settled with complaints ... ordering holds 1 cycle(s) across files, of 1"), `report` nao imprime aviso, e todos os comandos (`graph`, `plan`, `audit`, `report`, `emit`, `write`, `verify`) terminam com rc=0. A aresta com a razao `pin` aparece em `graph` e em `report`; o ciclo nomeado, com membros e razoes, so no `audit`.

### 3.10 Go-e2e: o teste HTTP espera a rota que chama

A regra `go-e2e` (config `{router, dirs, requests?}`, `router` com as mesmas chaves da `go-router`) acrescenta uma aresta `e2e` de um teste para a rota cuja URL ele chama. Ela le, em cada declaracao de teste (funcao, metodo ou instrucao) sob `dirs`, o literal de texto passado a uma chamada que pode ser uma requisicao: `/caminho` ou `METODO /caminho`, com `?` e `#` cortados. A rota vem da visao `Routes` da `go-router`, e a aresta aponta para a instrucao (ou funcao) que a registra e para cada instrucao ou funcao que a monta, subindo por todos os chamadores, em todos os niveis, com guarda de ciclo. O prefixo e o argumento constante `/...` da chamada de montagem (somado ao da chamada de montagem em cujo literal de funcao ela esta, se houver) em cada nivel. Uma rota de caminho literal vence uma de parametro (`/items/me` casa so a literal quando as duas existem), e uma chamada de rota sem palavra de metodo (`HandleFunc`) responde a qualquer metodo.

A razao `e2e` e mole: `birth.IsSoft` a conta, entao um ciclo que passa por ela e desfeito em vez de tratado como duro, mas em um caso simples o teste continua nascendo depois da rota (a espera e a mesma). Ela tambem nao faz de uma rota lida so por testes HTTP codigo de apoio de teste: `testplace.TestSupport` a ignora, e a feature da rota nao se funde em outro commit.

Sem `requests`, toda chamada conta, menos as de `testing`, `os`, `path`, `path/filepath`, `log`, `log/slog`, `fmt` e `strings`, e menos as que casam o `handle` ou o `mount` do proprio router (um servidor que o teste monta nao e requisicao). `requests` (lista de globs como os do router) estreita isso para so as chamadas listadas. `dirs` sao diretorios (`test/`); um glob e recusado ao carregar a regra.

Um resumo de uma linha, na primeira passada do indexador e nao a cada passada, e registrado em INFO (`internal/infra/langpack/idiom/e2e/report.go`). `cmd/redigit/main.go` fixa o nivel em WARN e nao ha flag para mudar isso, entao a linha so aparece quando a execucao achou declaracoes de teste e zero arestas; com uma aresta ou mais ela nao e impressa. Os numeros da linha sao os da primeira passada: nos fixtures da revisao o total de arestas impresso fica uma abaixo do grafo final (16 contra 17, 24 contra 25). No amigonimo (`2502d84`) ela saiu assim: `go-e2e: 32 test declarations in dirs, 32 without call facts (nested module?), 15 routes, 0 edges, 0 urls with no known route path`. Um segundo aviso, tambem uma vez, lista as URLs sem caminho de rota conhecido; ele e WARN e sempre sai quando a lista nao esta vazia; ele nunca diz que uma URL nao e servida. Um literal que comeca com `/` mas tem espaco ou quebra de linha nao e URL e nao entra na lista.

O que a regra nao ve: uma URL montada por um auxiliar, um campo de tabela, um `http://host/...` completo, um prefixo que nao e constante no ponto da chamada (o `GroupName()` do amigonimo), a variavel de um grupo aberto por atribuicao, e um teste em modulo aninhado sem fatos de chamada (a linha acima o conta como `without call facts`). Em modulo aninhado, hoje, quem funciona e a ancora de `tests/routepath.go`, que le o texto do teste e nao os fatos de chamada.

Tres casos acrescentam aresta errada ou deixam a rota certa sem aresta, sem aviso proprio:

- Um caminho nu `/x` num teste casa toda rota de grupo que termina em `/x`: no fixture da revisao `TestGroupBare` (`get("/x")`) ganha arestas para `Routes#6` e `Routes#8`, as duas rotas de grupo.
- O primeiro literal de texto que comeca com `/` passado a uma chamada de montagem e tomado como prefixo, mesmo quando nao e: `reg.Mount(mux, "/etc/conf")` gera `/etc/conf/reg`, que ganha arestas, e a rota realmente servida, `/reg`, fica sem aresta e entra na lista do aviso.
- Le-se so um nivel de `Route` aninhado: `Route("/n1", Route("/n2", GET("/deep")))` gera `/n2/deep`, que ganha aresta, e o caminho real `/n1/n2/deep` fica sem aresta e entra na lista.

## 4. As duas entradas

`base` deixa de ser a espinha dorsal e passa a ser evidencia opcional.

**Com historia de origem** (o caso do terectek): cada declaracao herda como
`base` a primeira posicao que a continha. O ponto fixo so empurra para frente.

**Sem historia** (arvore de um commit so): nao ha `base`. A ordem vem so da
dependencia, e ai o emissor por feature deixa de ser refinamento e passa a ser o
unico jeito de a historia nao sair puramente horizontal. Os clusters sao
ordenados topologicamente entre si; dentro do cluster, por dependencia.

Consequencia de projeto: `base` entra como um piso a mais, ao lado do piso de
modulo e do piso de leitor. Nao como semente privilegiada.

## 5. Verificacao: toda medida declara o que nao olha

O aprendizado mais transferivel do trabalho anterior e que onze vezes o numero
ruim veio do medidor, nao da arvore. Isso vira tipo, nao disciplina:

```go
// Measure e uma medicao e o que ela NAO olhou.
type Measure struct {
    Name     string
    Value    int
    Total    int
    Excluded []string // o que este numero nao enxerga
}
```

Uma medida sem `Excluded` preenchido nao passa na revisao. Os onze enganos
catalogados viram casos de teste:

- varredura que compila so o que o `go.work` daquele commit declara
- `vet` sem `-tags integration` (194 arquivos fora de toda prova)
- teste medido no arquivo em vez de na funcao
- casamento teste/funcao que pega o tipo em vez do metodo
- "primeiro uso" que ignora o uso no mesmo commit
- comparacao de shas onde o que precisa bater sao arvores
- prova de movibilidade contra um destino que depois mudou

E, do AtomicCommitBench: **replay e agrupamento sao relatados separadamente.**
A ferramenta nunca reporta so o invariante da arvore final.

## 6. Layout

Modulo unico, `github.com/wrapped-owls/redigit`, CGO desligado.

```
cmd/redigit/                 main fino
internal/
  bootstrap/                 raiz de composicao; unico lugar que escolhe adaptador
  domain/
    entities/                vocabulario compartilhado: Decl, Edge, Anchor, Plan, Measure
    apperr/                  contrato de erro
    usecases/
      indexing/              monta o grafo a partir das portas
      ordering/              ponto fixo, ciclos nomeados
      planning/              clusters de feature, kernel por camada
      rewriting/             render por commit, invariante da arvore
      auditing/              as medidas, cada uma com Excluded
  infra/
    lsp/                     JSON-RPC 2.0 + subconjunto do protocolo (agnostico)
    scip/                    leitor de indice SCIP (agnostico)
    langpack/go/             sintaxe, render e regras invisiveis do Go
    gitstore/                fast-import e leitura de historia
  transport/cli/             verbos
```

Dependencias: `golang.org/x/sync` (errgroup, exigido pela skill de
concorrencia), `github.com/dave/dst` (render), bindings SCIP e protobuf. Nada
mais.
