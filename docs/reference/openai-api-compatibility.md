# OpenAI API Compatibility

KubeAI provides an OpenAI API compatiblity layer.

## General:

### Models

```
GET /v1/models
```

* Lists all `kind: Model` object installed in teh Kubernetes API Server.


## Inference

### Text Generation

```
POST /v1/chat/completions
POST /v1/completions
```

* Supported for Models with `.spec.features: ["TextGeneration"]`.

### Embeddings

```
POST /v1/embeddings
```

* Supported for  Models with `.spec.features: ["TextEmbedding"]`.

### Reranking

```
POST /v1/vllm/rerank
```

* Supported for  Models with `.spec.features: ["Reranking"]`.

### Speech-to-Text

```
POST /v1/audio/transcriptions
```

* Supported for Models with `.spec.features: ["SpeechToText"]`.

### System One decisions (extension)

```text
POST /v1/systemone
```

* KubeAI URL: `/openai/v1/systemone`.
* Requires `.spec.features: ["SystemOne"]` and engine `OLlama`, `VLLM`,
  `LlamaCpp` or `SGLang`.
* The engine image must already serve `/v1/systemone` at its inference port.
* Requires a non-empty `model` string naming a KubeAI Model. Adapter selection
  is unsupported; other fields and responses are backend-defined.
* JSON and multipart bodies are preserved byte-for-byte. Multipart requests
  carry the JSON envelope in exactly one text form field named `request`.
* The complete body limit is 32 MiB. The existing proxy handles load balancing,
  scale-from-zero, retries and request accounting.
* See [Route System One requests](../how-to/configure-systemone-models.md) for
  configuration, discovery, examples, limitations and error behavior.

## OpenAI Client libaries
You can use the official OpenAI client libraries by setting the
`base_url` to the KubeAI endpoint.

For example, you can use the Python client like this:
```python
from openai import OpenAI
client = OpenAI(api_key="ignored",
                base_url="http://kubeai/openai/v1")
response = client.chat.completions.create(
  model="gemma2-2b-cpu",
  messages=[
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Who won the world series in 2020?"},
    {"role": "assistant", "content": "The Los Angeles Dodgers won the World Series in 2020."},
    {"role": "user", "content": "Where was it played?"}
  ]
)
```
