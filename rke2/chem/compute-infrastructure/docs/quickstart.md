# Khemeia — Getting Started

You'll need a terminal with `curl` and `jq` installed.

---

## Step 1: Sign in with GitHub

Go to **https://auth.hwcopeland.net** and click **Sign in with GitHub**. That's it — your account is created automatically.

After you sign in, let Hampton know your username so he can approve your access. Once approved, he'll send you two things:

- **Client ID** (e.g., `docking-controller`)
- **Client Secret** (a long random string)

Keep the client secret private. If you lose it, ask Hampton for a new one.

---

## Step 2: Get a token

Every time you want to use the API, grab a fresh token (they last 1 hour):

```bash
export CLIENT_ID="docking-controller"
export CLIENT_SECRET="paste-your-secret-here"

export TOKEN=$(curl -sf -X POST https://auth.hwcopeland.net/application/o/token/ \
  -d "grant_type=client_credentials" \
  -d "client_id=$CLIENT_ID" \
  -d "client_secret=$CLIENT_SECRET" \
  -d "scope=openid" | jq -r '.access_token')
```

Test it:
```bash
curl -sf -H "Authorization: Bearer $TOKEN" https://khemeia.hwcopeland.net/health
# Should print: {"status":"healthy", ...}
```

---

## Running a Quantum ESPRESSO calculation

### Write your input file

Here's a simple silicon SCF calculation:

```bash
cat > si.in << 'EOF'
&CONTROL
  calculation = 'scf'
  prefix = 'silicon'
  outdir = './tmp'
  pseudo_dir = './'
/
&SYSTEM
  ibrav = 2
  celldm(1) = 10.2
  nat = 2
  ntyp = 1
  ecutwfc = 30.0
/
&ELECTRONS
  mixing_beta = 0.7
  conv_thr = 1.0d-8
/
ATOMIC_SPECIES
  Si 28.086 Si.pbe-n-rrkjus_psl.1.0.0.UPF
ATOMIC_POSITIONS {alat}
  Si 0.00 0.00 0.00
  Si 0.25 0.25 0.25
K_POINTS {automatic}
  4 4 4 1 1 1
EOF
```

Pseudopotentials (the `.UPF` files) are downloaded automatically — you don't need to provide them.

### Submit it

```bash
JOB=$(curl -sf -X POST https://khemeia.hwcopeland.net/api/v1/qe/submit \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "$(jq -n --rawfile input si.in '{
    input_file: $input,
    executable: "pw.x",
    num_cpus: 2,
    memory_mb: 2048
  }')")

echo $JOB
# {"name":"qe-1774976667038212124","status":"Pending"}

JOB_NAME=$(echo $JOB | jq -r '.name')
```

### Wait for it

```bash
# Poll until done (usually < 1 minute for small systems)
while true; do
  STATUS=$(curl -sf -H "Authorization: Bearer $TOKEN" \
    https://khemeia.hwcopeland.net/api/v1/qe/jobs/$JOB_NAME | jq -r '.status')
  echo "Status: $STATUS"
  [ "$STATUS" = "Completed" ] || [ "$STATUS" = "Failed" ] && break
  sleep 10
done
```

### Get the results

```bash
curl -sf -H "Authorization: Bearer $TOKEN" \
  https://khemeia.hwcopeland.net/api/v1/qe/jobs/$JOB_NAME | jq '{
    status,
    total_energy,
    wall_time_sec
  }'
```

Output:
```json
{
  "status": "Completed",
  "total_energy": -22.83836927,
  "wall_time_sec": 0.26
}
```

To get the full QE output text:
```bash
curl -sf -H "Authorization: Bearer $TOKEN" \
  https://khemeia.hwcopeland.net/api/v1/qe/jobs/$JOB_NAME | jq -r '.output_file'
```

### List your jobs

```bash
curl -sf -H "Authorization: Bearer $TOKEN" \
  https://khemeia.hwcopeland.net/api/v1/qe/jobs | jq '.jobs[] | {name, status, total_energy}'
```

### Delete a job

```bash
curl -sf -X DELETE -H "Authorization: Bearer $TOKEN" \
  https://khemeia.hwcopeland.net/api/v1/qe/jobs/$JOB_NAME
```

---

## Running a molecular docking job

### Upload your compounds

Provide SMILES strings. Give them a group name (like a project name):

```bash
curl -sf -X POST https://khemeia.hwcopeland.net/api/v1/ligands \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '[
    {"compound_id": "aspirin", "smiles": "CC(=O)Oc1ccccc1C(O)=O", "source_db": "my-project"},
    {"compound_id": "caffeine", "smiles": "Cn1c(=O)c2c(ncn2C)n(C)c1=O", "source_db": "my-project"},
    {"compound_id": "ibuprofen", "smiles": "CC(C)Cc1ccc(cc1)C(C)C(O)=O", "source_db": "my-project"}
  ]'
```

### Prep them (converts 2D SMILES to 3D structures)

```bash
curl -sf -X POST https://khemeia.hwcopeland.net/api/v1/prep \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"source_db": "my-project"}'
```

This takes a minute or two. It generates 3D molecular structures using RDKit.

### Dock against a protein

Pick a PDB ID for your target protein. Example: `7jrn` is SARS-CoV-2 main protease.

```bash
DOCK=$(curl -sf -X POST https://khemeia.hwcopeland.net/api/v1/dockingjobs \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "pdbid": "7jrn",
    "ligand_db": "my-project",
    "native_ligand": "TTT",
    "ligands_chunk_size": 100
  }')

echo $DOCK
DOCK_NAME=$(echo $DOCK | jq -r '.name')
```

### Wait and check results

```bash
# Poll
while true; do
  STATUS=$(curl -sf -H "Authorization: Bearer $TOKEN" \
    https://khemeia.hwcopeland.net/api/v1/dockingjobs/$DOCK_NAME | jq -r '.status')
  echo "Status: $STATUS"
  [ "$STATUS" = "Completed" ] || [ "$STATUS" = "Failed" ] && break
  sleep 30
done

# Get binding energies
curl -sf -H "Authorization: Bearer $TOKEN" \
  https://khemeia.hwcopeland.net/api/v1/dockingjobs/$DOCK_NAME/results | jq
```

Output:
```json
{
  "result_count": 3,
  "best_affinity_kcal_mol": -6.1,
  "worst_affinity_kcal_mol": -5.0,
  "avg_affinity_kcal_mol": -5.55
}
```

More negative = better binding. Anything below -7.0 kcal/mol is a promising hit.

---

## Quick reference

| What | Command |
|------|---------|
| Health check | `curl https://khemeia.hwcopeland.net/health` |
| List QE jobs | `curl -H "Auth..." .../api/v1/qe/jobs` |
| List docking jobs | `curl -H "Auth..." .../api/v1/dockingjobs` |
| Submit QE | `POST .../api/v1/qe/submit` |
| Submit docking | `POST .../api/v1/dockingjobs` |
| Import compounds | `POST .../api/v1/ligands` |
| Prep compounds | `POST .../api/v1/prep` |

## Something broken?

Check `https://khemeia.hwcopeland.net/health`. If that works but your jobs fail, check the job status for an error message. Otherwise, message Hampton.
