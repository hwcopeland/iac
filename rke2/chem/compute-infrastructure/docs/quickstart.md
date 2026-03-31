# Khemeia Platform — Quickstart

## 1. Get an Account

1. Go to **https://auth.hwcopeland.net** and click **Sign Up** (or ask the admin to create your account)
2. Ask the admin to assign you to the **Docking Controller** application in Authentik
3. The admin will give you a **client ID** and **client secret** — save these securely

## 2. Get Your API Token

```bash
# Replace with your credentials
CLIENT_ID="your-client-id"
CLIENT_SECRET="your-client-secret"

# Get a JWT (valid for 1 hour, re-run when it expires)
TOKEN=$(curl -sf -X POST https://auth.hwcopeland.net/application/o/token/ \
  -d "grant_type=client_credentials" \
  -d "client_id=$CLIENT_ID" \
  -d "client_secret=$CLIENT_SECRET" \
  -d "scope=openid" | jq -r '.access_token')

echo $TOKEN
```

All API requests use this token:
```bash
curl -H "Authorization: Bearer $TOKEN" https://khemeia.hwcopeland.net/api/v1/...
```

## 3. What You Can Do

The platform runs two types of computational jobs:

| Service | What it does | Endpoint prefix |
|---------|-------------|-----------------|
| **Molecular Docking** | Dock compounds against protein targets (AutoDock Vina) | `/api/v1/dockingjobs` |
| **Quantum ESPRESSO** | First-principles DFT calculations (pw.x, ph.x, etc.) | `/api/v1/qe/` |

---

## Quantum ESPRESSO

### Submit a Calculation

Write your QE input file, then submit:

```bash
curl -X POST https://khemeia.hwcopeland.net/api/v1/qe/submit \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "$(jq -n --rawfile input your_input.in '{
    input_file: $input,
    executable: "pw.x",
    num_cpus: 2,
    memory_mb: 2048
  }')"
```

Response:
```json
{"name": "qe-1774976667038212124", "status": "Pending"}
```

### Check Status

```bash
curl -H "Authorization: Bearer $TOKEN" \
  https://khemeia.hwcopeland.net/api/v1/qe/jobs/qe-1774976667038212124
```

### Results

When `status` is `Completed`:
```json
{
  "status": "Completed",
  "total_energy": -22.83836927,
  "wall_time_sec": 0.26,
  "output_file": "... full QE output ..."
}
```

### List All Your Jobs

```bash
curl -H "Authorization: Bearer $TOKEN" \
  https://khemeia.hwcopeland.net/api/v1/qe/jobs
```

### Delete a Job

```bash
curl -X DELETE -H "Authorization: Bearer $TOKEN" \
  https://khemeia.hwcopeland.net/api/v1/qe/jobs/qe-1774976667038212124
```

### Pseudopotentials

Pseudopotentials (.UPF files) are auto-downloaded on first use. To pre-upload for faster runs:

```bash
curl -X POST https://khemeia.hwcopeland.net/api/v1/qe/pseudopotentials \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"filename\": \"Si.pbe-n-rrkjus_psl.1.0.0.UPF\",
    \"content_b64\": \"$(base64 -w0 Si.pbe-n-rrkjus_psl.1.0.0.UPF)\",
    \"element\": \"Si\",
    \"functional\": \"PBE\"
  }"
```

### Supported Executables

| Executable | Use |
|-----------|-----|
| `pw.x` | SCF, relaxation, MD (default) |
| `ph.x` | Phonon calculations |
| `bands.x` | Band structure |
| `pp.x` | Post-processing |
| `dos.x` | Density of states |
| `projwfc.x` | Projected DOS |

### Resource Limits

| Parameter | Default | Max |
|-----------|---------|-----|
| `num_cpus` | 1 | 20 |
| `memory_mb` | 2048 | 32768 |
| Timeout | 4 hours | — |

---

## Molecular Docking

### Import Compounds

Upload compounds as SMILES:
```bash
curl -X POST https://khemeia.hwcopeland.net/api/v1/ligands \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '[
    {"compound_id": "aspirin", "smiles": "CC(=O)Oc1ccccc1C(O)=O", "source_db": "my-compounds"},
    {"compound_id": "ibuprofen", "smiles": "CC(C)Cc1ccc(cc1)C(C)C(O)=O", "source_db": "my-compounds"}
  ]'
```

### Prep Ligands (SMILES → 3D PDBQT)

```bash
curl -X POST https://khemeia.hwcopeland.net/api/v1/prep \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"source_db": "my-compounds"}'
```

Wait a few minutes for RDKit to generate 3D structures.

### Run Docking

```bash
curl -X POST https://khemeia.hwcopeland.net/api/v1/dockingjobs \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "pdbid": "7jrn",
    "ligand_db": "my-compounds",
    "native_ligand": "TTT",
    "ligands_chunk_size": 100
  }'
```

### Check Results

```bash
# Job status
curl -H "Authorization: Bearer $TOKEN" \
  https://khemeia.hwcopeland.net/api/v1/dockingjobs/docking-XXXX

# Binding energies
curl -H "Authorization: Bearer $TOKEN" \
  https://khemeia.hwcopeland.net/api/v1/dockingjobs/docking-XXXX/results
```

Response:
```json
{
  "result_count": 2,
  "best_affinity_kcal_mol": -6.1,
  "worst_affinity_kcal_mol": -5.0,
  "avg_affinity_kcal_mol": -5.55
}
```

---

## Job Ownership

All jobs are tagged with your username. The admin can see all jobs; you can see yours.

## Need Help?

Contact the platform admin or check the API health:
```bash
curl https://khemeia.hwcopeland.net/health
```
