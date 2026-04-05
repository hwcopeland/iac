#!/usr/bin/env bash
# benchmark.sh — Docking parallelism benchmark tool
#
# Tests different chunk-size configurations (and optionally parallel job counts)
# against the docking controller API, measuring end-to-end throughput including
# pod startup, AutoDock Vina execution, and staging drain.
#
# Usage:
#   ./benchmark.sh [OPTIONS]
#   ./benchmark.sh --parallel [OPTIONS]
#
# Examples:
#   ./benchmark.sh                                    # default chunk sizes
#   ./benchmark.sh --configs "50 100 250 500"         # custom chunk sizes
#   ./benchmark.sh --parallel --concurrency "1 2 4"   # parallel job test
#   ./benchmark.sh --dry-run                          # show plan without running

set -euo pipefail

# ---------------------------------------------------------------------------
# Defaults
# ---------------------------------------------------------------------------
CONTROLLER_URL="${CONTROLLER_URL:-http://localhost:18080}"
SOURCE_DB="${SOURCE_DB:-mpro-test}"
PDBID="${PDBID:-7jrn}"
NATIVE_LIGAND="${NATIVE_LIGAND:-TTT}"
TOTAL_LIGANDS="${TOTAL_LIGANDS:-}"
POLL_INTERVAL=10    # seconds between status polls
CLEANUP=true
DRY_RUN=false
MODE="chunk"        # "chunk" or "parallel"

# Chunk-size mode defaults
declare -a CONFIGS=(50 100 250 500 1000)

# Parallel mode defaults
PARALLEL_CHUNK_SIZE=100
declare -a CONCURRENCY_LEVELS=(1 2 4)

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
log()  { echo "[$(date +%H:%M:%S)] $*"; }
err()  { echo "[$(date +%H:%M:%S)] ERROR: $*" >&2; }
die()  { err "$@"; exit 1; }

has_kubectl() { command -v kubectl &>/dev/null; }
has_jq()      { command -v jq &>/dev/null; }

# Require jq for JSON parsing
check_deps() {
    if ! has_jq; then
        die "jq is required but not found. Install it: brew install jq / apt install jq"
    fi
}

# HTTP helpers — all return raw JSON; caller checks for errors.
api_get()    { curl -sf --max-time 30 "$CONTROLLER_URL$1" 2>/dev/null; }
api_post()   { curl -sf --max-time 30 -X POST -H "Content-Type: application/json" -d "$2" "$CONTROLLER_URL$1" 2>/dev/null; }
api_delete() { curl -sf --max-time 30 -X DELETE "$CONTROLLER_URL$1" 2>/dev/null || true; }

# Format seconds into human-readable "Xm Ys"
fmt_duration() {
    local secs=$1
    if (( secs >= 60 )); then
        printf "%dm %ds" $((secs / 60)) $((secs % 60))
    else
        printf "%ds" "$secs"
    fi
}

# ---------------------------------------------------------------------------
# Usage / Help
# ---------------------------------------------------------------------------
usage() {
    cat <<'HELP'
Docking Parallelism Benchmark

Measures docking throughput under different configurations by submitting jobs
to the docking controller API and timing end-to-end completion.

USAGE
  ./benchmark.sh [OPTIONS]

MODES
  (default)     Test different chunk sizes sequentially.
  --parallel    Test concurrent job submissions with a fixed chunk size.

OPTIONS
  --url URL             Controller URL          (default: $CONTROLLER_URL or http://localhost:18080)
  --source-db DB        Ligand source database  (default: $SOURCE_DB or mpro-test)
  --pdbid ID            Receptor PDB ID         (default: $PDBID or 7jrn)
  --native-ligand ID    Native ligand ID        (default: $NATIVE_LIGAND or TTT)
  --total-ligands N     Override ligand count   (default: auto-detect from controller)
  --configs "N N N"     Chunk sizes to test     (default: "50 100 250 500 1000")
  --poll-interval N     Seconds between polls   (default: 10)
  --no-cleanup          Skip DELETE after test   (keeps workflows for inspection)
  --dry-run             Print the test plan without submitting any jobs
  --help, -h            Show this help

PARALLEL MODE OPTIONS
  --parallel            Enable parallel mode: tests N concurrent jobs
  --concurrency "N N"   Concurrency levels      (default: "1 2 4")
  --chunk-size N        Fixed chunk size         (default: 100)

ENVIRONMENT VARIABLES
  CONTROLLER_URL, SOURCE_DB, PDBID, NATIVE_LIGAND, TOTAL_LIGANDS
  can all be set via environment instead of flags.

EXAMPLES
  # Test chunk sizes 50, 100, 250 against mpro-test
  ./benchmark.sh --configs "50 100 250"

  # Test how many parallel jobs the cluster handles
  ./benchmark.sh --parallel --concurrency "1 2 4 8" --chunk-size 50

  # Dry run to see what would happen
  ./benchmark.sh --dry-run --configs "100 500"

OUTPUT
  Prints a summary table with wall time, ligands/second, and seconds/ligand
  for each configuration tested.
HELP
    exit 0
}

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --help|-h)       usage ;;
            --url)           CONTROLLER_URL="$2"; shift 2 ;;
            --source-db)     SOURCE_DB="$2"; shift 2 ;;
            --pdbid)         PDBID="$2"; shift 2 ;;
            --native-ligand) NATIVE_LIGAND="$2"; shift 2 ;;
            --total-ligands) TOTAL_LIGANDS="$2"; shift 2 ;;
            --configs)       IFS=' ' read -ra CONFIGS <<< "$2"; shift 2 ;;
            --poll-interval) POLL_INTERVAL="$2"; shift 2 ;;
            --no-cleanup)    CLEANUP=false; shift ;;
            --dry-run)       DRY_RUN=true; shift ;;
            --parallel)      MODE="parallel"; shift ;;
            --concurrency)   IFS=' ' read -ra CONCURRENCY_LEVELS <<< "$2"; shift 2 ;;
            --chunk-size)    PARALLEL_CHUNK_SIZE="$2"; shift 2 ;;
            *)               die "Unknown option: $1 (try --help)" ;;
        esac
    done
}

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------
preflight() {
    log "Checking controller at $CONTROLLER_URL ..."
    local health
    health=$(api_get "/health") || die "Controller unreachable at $CONTROLLER_URL"
    echo "$health" | jq -e '.status == "healthy"' &>/dev/null \
        || die "Controller health check failed: $health"
    log "Controller is healthy."

    # Auto-detect total ligands if not specified
    if [[ -z "$TOTAL_LIGANDS" ]]; then
        log "Detecting ligand count for source_db=$SOURCE_DB ..."
        # Submit a dry probe: try to create a job to get the ligand count error,
        # or list existing workflows. Since the controller validates ligands exist
        # on POST, we can use the list endpoint and check existing data.
        # The simplest approach: try a GET on an existing workflow or just proceed
        # with the first config and observe batch_count * chunk_size.
        log "TOTAL_LIGANDS not set; will compute from first run's batch_count."
        TOTAL_LIGANDS="auto"
    fi
}

# ---------------------------------------------------------------------------
# Submit a docking job; prints the workflow name on success
# ---------------------------------------------------------------------------
submit_job() {
    local chunk_size=$1
    local payload
    payload=$(jq -n \
        --arg pdbid "$PDBID" \
        --arg ligand_db "$SOURCE_DB" \
        --arg native_ligand "$NATIVE_LIGAND" \
        --argjson chunk_size "$chunk_size" \
        '{pdbid: $pdbid, ligand_db: $ligand_db, native_ligand: $native_ligand, ligands_chunk_size: $chunk_size}')

    local resp
    resp=$(api_post "/api/v1/dockingjobs" "$payload") \
        || die "Failed to submit job (chunk_size=$chunk_size)"

    local name
    name=$(echo "$resp" | jq -r '.name // empty')
    if [[ -z "$name" ]]; then
        die "No workflow name in response: $resp"
    fi
    echo "$name"
}

# ---------------------------------------------------------------------------
# Poll until Completed or Failed; prints final status JSON
# ---------------------------------------------------------------------------
wait_for_completion() {
    local name=$1
    local status batch_count completed_batches resp

    while true; do
        resp=$(api_get "/api/v1/dockingjobs/$name") \
            || { err "Failed to poll $name"; sleep "$POLL_INTERVAL"; continue; }

        status=$(echo "$resp" | jq -r '.status')
        batch_count=$(echo "$resp" | jq -r '.batch_count')
        completed_batches=$(echo "$resp" | jq -r '.completed_batches')

        case "$status" in
            Completed)
                log "  $name: COMPLETED ($completed_batches/$batch_count batches)"
                echo "$resp"
                return 0
                ;;
            Failed)
                local msg
                msg=$(echo "$resp" | jq -r '.message // "unknown error"')
                err "  $name: FAILED — $msg"
                echo "$resp"
                return 1
                ;;
            *)
                log "  $name: $status ($completed_batches/$batch_count batches) ..."
                ;;
        esac

        sleep "$POLL_INTERVAL"
    done
}

# ---------------------------------------------------------------------------
# Get result stats for a workflow
# ---------------------------------------------------------------------------
get_results() {
    local name=$1
    api_get "/api/v1/dockingjobs/$name/results"
}

# ---------------------------------------------------------------------------
# Capture peak pod resource usage via kubectl top (best-effort)
# ---------------------------------------------------------------------------
capture_pod_resources() {
    local name=$1
    if ! has_kubectl; then
        return
    fi
    log "  Capturing pod resources for $name ..."
    kubectl top pods -l "docking.khemia.io/parent-job=$name" --no-headers 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Delete a workflow (cleanup)
# ---------------------------------------------------------------------------
cleanup_workflow() {
    local name=$1
    if [[ "$CLEANUP" != true ]]; then
        return
    fi
    log "  Cleaning up $name ..."
    api_delete "/api/v1/dockingjobs/$name"
}

# ---------------------------------------------------------------------------
# Chunk-size benchmark mode
# ---------------------------------------------------------------------------
run_chunk_benchmark() {
    local -a results_names=()
    local -a results_chunk=()
    local -a results_batches=()
    local -a results_wall=()
    local -a results_lps=()     # ligands per second
    local -a results_spl=()     # seconds per ligand
    local -a results_count=()
    local -a results_status=()
    local detected_total=""

    local total_configs=${#CONFIGS[@]}
    log "Starting chunk-size benchmark: ${total_configs} configuration(s)"
    log "  Configs: ${CONFIGS[*]}"
    log "  Source DB: $SOURCE_DB"
    log "  PDB ID: $PDBID"
    echo ""

    for i in "${!CONFIGS[@]}"; do
        local chunk=${CONFIGS[$i]}
        local run_num=$((i + 1))

        log "[$run_num/$total_configs] Testing chunk_size=$chunk ..."

        # Submit
        local name
        name=$(submit_job "$chunk")
        log "  Submitted: $name"

        local t_start
        t_start=$(date +%s)

        # Poll
        local final_json
        local job_status="Failed"
        if final_json=$(wait_for_completion "$name"); then
            job_status="OK"
        fi

        local t_end
        t_end=$(date +%s)
        local wall=$((t_end - t_start))

        # Capture pod resources while they might still exist
        capture_pod_resources "$name"

        # Fetch results
        local result_count=0
        local batch_count=0
        batch_count=$(echo "$final_json" | jq -r '.batch_count // 0')

        if [[ "$job_status" == "OK" ]]; then
            local results_json
            results_json=$(get_results "$name")
            result_count=$(echo "$results_json" | jq -r '.result_count // 0')
        fi

        # Auto-detect total ligands from first successful run
        if [[ "$TOTAL_LIGANDS" == "auto" && "$result_count" -gt 0 ]]; then
            detected_total="$result_count"
            TOTAL_LIGANDS="$result_count"
            log "  Auto-detected TOTAL_LIGANDS=$TOTAL_LIGANDS"
        fi

        # Compute throughput
        local lps="N/A" spl="N/A"
        if [[ "$result_count" -gt 0 && "$wall" -gt 0 ]]; then
            lps=$(awk "BEGIN {printf \"%.3f\", $result_count / $wall}")
            spl=$(awk "BEGIN {printf \"%.3f\", $wall / $result_count}")
        fi

        # Store results
        results_names+=("$name")
        results_chunk+=("$chunk")
        results_batches+=("$batch_count")
        results_wall+=("$wall")
        results_lps+=("$lps")
        results_spl+=("$spl")
        results_count+=("$result_count")
        results_status+=("$job_status")

        log "  Done: wall=$(fmt_duration "$wall"), results=$result_count, l/s=$lps"
        echo ""

        # Cleanup
        cleanup_workflow "$name"
    done

    # Summary table
    print_chunk_summary
}

# ---------------------------------------------------------------------------
# Print chunk-size summary table
# ---------------------------------------------------------------------------
print_chunk_summary() {
    echo ""
    echo "BENCHMARK RESULTS -- PDBID: $PDBID, Source: $SOURCE_DB"
    if [[ "$TOTAL_LIGANDS" != "auto" && -n "$TOTAL_LIGANDS" ]]; then
        echo "Total Ligands: $TOTAL_LIGANDS"
    fi
    printf '=%.0s' {1..72}; echo ""
    printf "%-12s | %-7s | %-10s | %-9s | %-8s | %-7s | %s\n" \
        "Chunk Size" "Batches" "Wall Time" "Results" "Ligands/s" "s/Lig" "Status"
    printf -- '-%.0s' {1..72}; echo ""

    for i in "${!results_chunk[@]}"; do
        printf "%-12s | %-7s | %-10s | %-9s | %-9s | %-7s | %s\n" \
            "${results_chunk[$i]}" \
            "${results_batches[$i]}" \
            "$(fmt_duration "${results_wall[$i]}")" \
            "${results_count[$i]}" \
            "${results_lps[$i]}" \
            "${results_spl[$i]}" \
            "${results_status[$i]}"
    done

    printf '=%.0s' {1..72}; echo ""
    echo ""
    if has_kubectl; then
        echo "Note: kubectl was available; pod resource snapshots were captured above."
    else
        echo "Note: kubectl not found; pod resource data was not captured."
    fi
    echo ""
}

# ---------------------------------------------------------------------------
# Parallel benchmark mode
# ---------------------------------------------------------------------------
run_parallel_benchmark() {
    local -a results_concurrency=()
    local -a results_wall=()
    local -a results_total_results=()
    local -a results_lps=()
    local -a results_spl=()
    local -a results_status=()

    local total_levels=${#CONCURRENCY_LEVELS[@]}
    log "Starting parallel benchmark: ${total_levels} concurrency level(s)"
    log "  Concurrency: ${CONCURRENCY_LEVELS[*]}"
    log "  Chunk size (fixed): $PARALLEL_CHUNK_SIZE"
    log "  Source DB: $SOURCE_DB"
    log "  PDB ID: $PDBID"
    echo ""

    for i in "${!CONCURRENCY_LEVELS[@]}"; do
        local n=${CONCURRENCY_LEVELS[$i]}
        local run_num=$((i + 1))

        log "[$run_num/$total_levels] Testing concurrency=$n ..."

        # Submit N jobs concurrently
        local -a job_names=()
        for j in $(seq 1 "$n"); do
            local name
            name=$(submit_job "$PARALLEL_CHUNK_SIZE")
            job_names+=("$name")
            log "  Submitted [$j/$n]: $name"
        done

        local t_start
        t_start=$(date +%s)

        # Wait for all N jobs to finish
        local all_ok=true
        local -a completion_statuses=()
        for name in "${job_names[@]}"; do
            local final_json
            if final_json=$(wait_for_completion "$name"); then
                completion_statuses+=("OK")
            else
                completion_statuses+=("FAILED")
                all_ok=false
            fi
        done

        local t_end
        t_end=$(date +%s)
        local wall=$((t_end - t_start))

        # Capture pod resources for all jobs
        for name in "${job_names[@]}"; do
            capture_pod_resources "$name"
        done

        # Aggregate results across all N jobs
        local total_results=0
        for name in "${job_names[@]}"; do
            local rjson
            rjson=$(get_results "$name" 2>/dev/null || echo '{"result_count":0}')
            local rc
            rc=$(echo "$rjson" | jq -r '.result_count // 0')
            total_results=$((total_results + rc))
        done

        # Compute throughput
        local lps="N/A" spl="N/A"
        if [[ "$total_results" -gt 0 && "$wall" -gt 0 ]]; then
            lps=$(awk "BEGIN {printf \"%.3f\", $total_results / $wall}")
            spl=$(awk "BEGIN {printf \"%.3f\", $wall / $total_results}")
        fi

        local status="OK"
        if [[ "$all_ok" != true ]]; then
            status="PARTIAL"
        fi

        # Store results
        results_concurrency+=("$n")
        results_wall+=("$wall")
        results_total_results+=("$total_results")
        results_lps+=("$lps")
        results_spl+=("$spl")
        results_status+=("$status")

        log "  Done: wall=$(fmt_duration "$wall"), total_results=$total_results, l/s=$lps"
        echo ""

        # Cleanup
        for name in "${job_names[@]}"; do
            cleanup_workflow "$name"
        done
    done

    # Summary table
    print_parallel_summary
}

# ---------------------------------------------------------------------------
# Print parallel summary table
# ---------------------------------------------------------------------------
print_parallel_summary() {
    echo ""
    echo "PARALLEL BENCHMARK RESULTS -- PDBID: $PDBID, Source: $SOURCE_DB"
    echo "Fixed chunk size: $PARALLEL_CHUNK_SIZE"
    printf '=%.0s' {1..72}; echo ""
    printf "%-12s | %-10s | %-12s | %-9s | %-7s | %s\n" \
        "Concurrency" "Wall Time" "Total Results" "Ligands/s" "s/Lig" "Status"
    printf -- '-%.0s' {1..72}; echo ""

    for i in "${!results_concurrency[@]}"; do
        printf "%-12s | %-10s | %-12s | %-9s | %-7s | %s\n" \
            "${results_concurrency[$i]}" \
            "$(fmt_duration "${results_wall[$i]}")" \
            "${results_total_results[$i]}" \
            "${results_lps[$i]}" \
            "${results_spl[$i]}" \
            "${results_status[$i]}"
    done

    printf '=%.0s' {1..72}; echo ""
    echo ""
    if has_kubectl; then
        echo "Note: kubectl was available; pod resource snapshots were captured above."
    else
        echo "Note: kubectl not found; pod resource data was not captured."
    fi
    echo ""
}

# ---------------------------------------------------------------------------
# Dry-run output
# ---------------------------------------------------------------------------
print_dry_run() {
    echo ""
    echo "=== DRY RUN ==="
    echo ""
    echo "Controller:     $CONTROLLER_URL"
    echo "Source DB:       $SOURCE_DB"
    echo "PDB ID:         $PDBID"
    echo "Native Ligand:  $NATIVE_LIGAND"
    echo "Total Ligands:  ${TOTAL_LIGANDS:-auto-detect}"
    echo "Poll Interval:  ${POLL_INTERVAL}s"
    echo "Cleanup:        $CLEANUP"
    echo "kubectl:        $(has_kubectl && echo "available" || echo "not found")"
    echo ""

    if [[ "$MODE" == "chunk" ]]; then
        echo "Mode: Chunk-size benchmark"
        echo "Configs: ${CONFIGS[*]}"
        echo ""
        echo "Plan:"
        for i in "${!CONFIGS[@]}"; do
            local chunk=${CONFIGS[$i]}
            echo "  $((i + 1)). POST /api/v1/dockingjobs  {ligand_db: \"$SOURCE_DB\", pdbid: \"$PDBID\", ligands_chunk_size: $chunk}"
            echo "     -> Poll GET /api/v1/dockingjobs/{name} until Completed/Failed"
            echo "     -> GET /api/v1/dockingjobs/{name}/results"
            if [[ "$CLEANUP" == true ]]; then
                echo "     -> DELETE /api/v1/dockingjobs/{name}"
            fi
        done
    else
        echo "Mode: Parallel benchmark"
        echo "Chunk size (fixed): $PARALLEL_CHUNK_SIZE"
        echo "Concurrency levels: ${CONCURRENCY_LEVELS[*]}"
        echo ""
        echo "Plan:"
        for i in "${!CONCURRENCY_LEVELS[@]}"; do
            local n=${CONCURRENCY_LEVELS[$i]}
            echo "  $((i + 1)). Submit $n concurrent jobs (chunk_size=$PARALLEL_CHUNK_SIZE)"
            echo "     -> POST /api/v1/dockingjobs x$n"
            echo "     -> Poll all $n until Completed/Failed"
            echo "     -> GET results for each, aggregate throughput"
            if [[ "$CLEANUP" == true ]]; then
                echo "     -> DELETE all $n workflows"
            fi
        done
    fi

    echo ""
    echo "No jobs were submitted."
    echo ""
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
    parse_args "$@"
    check_deps

    if [[ "$DRY_RUN" == true ]]; then
        print_dry_run
        exit 0
    fi

    preflight

    if [[ "$MODE" == "chunk" ]]; then
        run_chunk_benchmark
    else
        run_parallel_benchmark
    fi
}

main "$@"
