# Configure embedding models

KubeAI supports the following engines for text embedding models:

- Infinity
- vLLM
- Ollama

Infinity supports any HuggingFace models listed as text-embedding. See the [models, reranking or clip models on huggingface](https://huggingface.co/models?other=text-embeddings-inference&sort=trending) for reference.


## Install BAAI/bge-small-en-v1.5 model using Infinity

Create a file named `kubeai-models.yaml` with the following content:

```yaml
catalog:
  bge-embed-text-cpu:
    enabled: true
    features: ["TextEmbedding"]
    owner: baai
    url: "hf://BAAI/bge-small-en-v1.5"
    engine: Infinity
    resourceProfile: cpu:1
    minReplicas: 1
```

Apply the kubeai-models helm chart:

```bash
helm install kubeai-models kubeai/models -f ./kubeai-models.yaml
```

Once the pod is ready, you can use the OpenAI Python SDK to interact with the model:

```python
from openai import OpenAI
# Assumes port-forward of kubeai service to localhost:8000.
# Reranking endpoints are exposed under /v1/vllm.
client = OpenAI(api_key="ignored", base_url="http://localhost:8000/openai/v1/vllm")
response = client.embeddings.create(
    input="Your text goes here.",
    model="bge-embed-text-cpu"
)
```

## Install BAAI/bge-reranker-base model using Infinity

Create a file named `kubeai-models.yaml` with the following content:

```yaml
catalog:
  bge-rerank-text-cpu:
    enabled: true
    features: ["Reranking"]
    owner: baai
    url: "hf://BAAI/bge-reranker-base"
    engine: Infinity
    resourceProfile: cpu:1
    minReplicas: 1
```

Apply the kubeai-models helm chart:

```bash
helm install kubeai-models kubeai/models -f ./kubeai-models.yaml
```

You can then call the rerank API:

```python
from openai import OpenAI
# Assumes port-forward of kubeai service to localhost:8000.
client = OpenAI(api_key="ignored", base_url="http://localhost:8000/openai/v1/vllm")
response = client.rerank(
    model="bge-rerank-text-cpu",
    query="What is Kubernetes?",
    documents=["Kubernetes is an orchestration platform.", "Other text"],
)
```

