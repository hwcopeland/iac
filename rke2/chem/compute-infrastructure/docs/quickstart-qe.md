# Quantum ESPRESSO on Khemia — Quickstart

## API Endpoint

```
https://khemeia.hwcopeland.net/api/v1/qe/
```

## Authentication

All external API requests require a JWT token from Authentik. Internal cluster requests (pods, E2E tests) are exempt.

### Get a Token

```bash
# Client credentials grant (no browser needed)
TOKEN=$(curl -sf -X POST https://auth.hwcopeland.net/application/o/token/ \
  -d "grant_type=client_credentials" \
  -d "client_id=docking-controller" \
  -d "client_secret=<your-client-secret>" \
  -d "scope=openid" | jq -r '.access_token')

# Token is valid for 1 hour
echo $TOKEN
```

Your client secret is in Bitwarden under `docking-controller-oidc` in the `k8s-secrets` folder.

### Use the Token

Add it to every request:

```bash
curl -H "Authorization: Bearer $TOKEN" https://khemeia.hwcopeland.net/api/v1/qe/jobs
```

## Running a Calculation

### 1. Write your QE input file

```bash
cat > si_scf.in << 'EOF'
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

### 2. Submit the job

```bash
curl -X POST https://khemeia.hwcopeland.net/api/v1/qe/submit \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "$(jq -n --rawfile input si_scf.in '{
    input_file: $input,
    executable: "pw.x",
    num_cpus: 2,
    memory_mb: 2048
  }')"
```

Response:
```json
{"name": "qe-1774969295168344935", "status": "Pending"}
```

### 3. Check status

```bash
curl -H "Authorization: Bearer $TOKEN" \
  https://khemeia.hwcopeland.net/api/v1/qe/jobs/qe-1774969295168344935
```

### 4. Get results

When status is `Completed`:
```json
{
  "name": "qe-1774969295168344935",
  "status": "Completed",
  "executable": "pw.x",
  "total_energy": -22.83836927,
  "wall_time_sec": 0.26,
  "output_file": "... full QE output ..."
}
```

## Pseudopotentials

Pseudopotentials (.UPF files) are auto-downloaded from the QE server on first use. For faster runs, upload them to the database:

```bash
# Upload a pseudopotential
curl -X POST https://khemeia.hwcopeland.net/api/v1/qe/pseudopotentials \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"filename\": \"Si.pbe-n-rrkjus_psl.1.0.0.UPF\",
    \"content_b64\": \"$(base64 -w0 Si.pbe-n-rrkjus_psl.1.0.0.UPF)\",
    \"element\": \"Si\",
    \"functional\": \"PBE\"
  }"

# List stored pseudopotentials
curl -H "Authorization: Bearer $TOKEN" \
  https://khemeia.hwcopeland.net/api/v1/qe/pseudopotentials
```

Stored pseudopotentials are automatically included in the job's input — no download needed.

## API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/qe/submit` | Submit a calculation |
| `GET` | `/api/v1/qe/jobs` | List all jobs |
| `GET` | `/api/v1/qe/jobs/{name}` | Get job details + output |
| `DELETE` | `/api/v1/qe/jobs/{name}` | Delete a job |
| `POST` | `/api/v1/qe/pseudopotentials` | Upload a pseudopotential |
| `GET` | `/api/v1/qe/pseudopotentials` | List pseudopotentials |

## Supported Executables

The `executable` field accepts any QE binary:
- `pw.x` — plane-wave SCF, relaxation, MD (default)
- `ph.x` — phonon calculations
- `bands.x` — band structure
- `pp.x` — post-processing
- `dos.x` — density of states
- `projwfc.x` — projected DOS

## Resource Limits

| Parameter | Default | Max |
|-----------|---------|-----|
| `num_cpus` | 1 | 20 |
| `memory_mb` | 2048 | 32768 |
| Timeout | 4 hours | — |

## Job Ownership

All jobs are tagged with your Authentik username (`submitted_by` field). You can see this in the job details.
