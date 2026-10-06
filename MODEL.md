# Il modello che separa il basso

*English follows the Italian.*

C_bass non contiene il modello: lo scarica la prima volta e lo tiene nella cache dell'utente. Questa pagina dice cos'è, da dove viene e a quali condizioni lo si può usare, perché sono diverse da quelle del codice.

## Cos'è

`htdemucs`, la rete «Hybrid Transformer Demucs» di Alexandre Défossez e colleghi (Meta AI), che divide una registrazione in batteria, basso, voce e resto. C_bass ne usa il basso, e la somma delle altre tre parti come «il resto».

- Articolo: Rouard, Massa, Défossez, *Hybrid Transformers for Music Source Separation*, 2022.
- Codice e pesi originali: <https://github.com/facebookresearch/demucs>.

## Da dove lo prende C_bass

| | |
|---|---|
| File | `htdemucs.onnx`, 174 MB: la rete esportata in formato ONNX |
| Indirizzo | `https://basso.massimodanieli.com/assets/separator/htdemucs.onnx` (il sito di Manico, che serve lo stesso file ai browser) |
| Impronta SHA-256 | `da9e5101ee0804d04933974b59d8aae9c862e80e14f2f24e7c74cae76bdbe748` |

Il programma controlla l'impronta dopo lo scaricamento e rifiuta un file diverso. Indirizzo e impronta sono in `internal/provision/provision.go`.

Per usare un altro file (una copia locale, o un altro modello con gli stessi ingressi e le stesse uscite): `CBASS_MODEL=/percorso/del/file.onnx`. Per un'altra libreria ONNX Runtime: `CBASS_ORT_LIB`.

## A quali condizioni

Il **codice** di Demucs è pubblicato con licenza MIT.

I **pesi** sono un'altra cosa. Sono stati allenati, tra l'altro, su MUSDB18-HQ, una raccolta di brani concessa per uso didattico e di ricerca, non commerciale. Se e quanto quelle condizioni seguano i pesi è una questione che gli autori non hanno chiuso e su cui chi scrive non è in grado di dare una risposta legale.

Quindi:

- C_bass è distribuito gratis e non vende né il modello né quello che il modello produce;
- chi volesse farne un uso commerciale deve chiarire la questione per conto suo, o usare un modello di cui ha i diritti, con `CBASS_MODEL`;
- la licenza GPL del repository copre il codice di C_bass, non i pesi.

## Cosa esce dal modello

Il basso separato e il resto restano nella cartella dei brani dell'utente. Sono derivati della registrazione di partenza e ne seguono i diritti: C_bass non li manda da nessuna parte.

---

## English

C_bass does not contain the model: it downloads it on first use and keeps it in the user's cache.

**What it is.** `htdemucs`, the Hybrid Transformer Demucs network by Alexandre Défossez and colleagues (Meta AI), which splits a recording into drums, bass, vocals and the rest. C_bass uses the bass, and the sum of the other three as "the rest". Paper: Rouard, Massa, Défossez, *Hybrid Transformers for Music Source Separation*, 2022. Original code and weights: <https://github.com/facebookresearch/demucs>.

**Where C_bass gets it.** `htdemucs.onnx`, 174 MB, the network exported to ONNX, from `https://basso.massimodanieli.com/assets/separator/htdemucs.onnx` (Manico's site, which serves the same file to browsers), SHA-256 `da9e5101ee0804d04933974b59d8aae9c862e80e14f2f24e7c74cae76bdbe748`. The hash is checked after the download; address and hash are in `internal/provision/provision.go`. To use another file, set `CBASS_MODEL=/path/to/file.onnx`; for another ONNX Runtime library, `CBASS_ORT_LIB`.

**On what terms.** The Demucs *code* is MIT-licensed. The *weights* are another matter: they were trained on MUSDB18-HQ among other material, a collection licensed for educational and research use, not commercial use. Whether and how far those terms follow the weights is a question the authors have not settled and the writer cannot answer as a lawyer would. So: C_bass is given away and sells neither the model nor what it produces; anyone wanting commercial use must settle the question themselves, or use a model they have the rights to through `CBASS_MODEL`; the repository's GPL covers the code of C_bass, not the weights.

**What comes out of the model.** The separated bass and the rest stay in the user's recordings folder. They derive from the original recording and follow its rights: C_bass sends them nowhere.
