#!/bin/bash
# verify_stats.sh - Independent verification of stats calculations using bc
# Uses the same 31-number dataset from stats_test.go
# Created on MacOS Sequoia 15.7.3
# COMPATIBILITY: Must work with bash 5.2+ (GitHub Actions requirement)
#
# The program is read through its JSON output (-j) and parsed with jq, so this
# script compares numbers against numbers rather than scraping padded text.

set -e

# Dataset (same as testData in stats_test.go)
DATA="5 10 15.5 20 25 30 35 40 45 50 55 60 65 70 75.25 80 85 90 95 100 12.5 37.5 62.5 87.5 50 50 50 3 150 7.75 42"
COUNT=31

for tool in bc jq; do
    if ! command -v "$tool" &> /dev/null; then
        echo "Error: required tool '$tool' not found on PATH"
        exit 1
    fi
done

TMPFILES=()
cleanup() {
    if [[ ${#TMPFILES[@]} -gt 0 ]]; then
        rm -f "${TMPFILES[@]}"
    fi
}
trap cleanup EXIT

# new_tempfile writes the given whitespace-separated values one per line and
# echoes the path. All temp files are removed by the EXIT trap.
new_tempfile() {
    local path
    path=$(mktemp)
    TMPFILES+=("$path")
    echo "$1" | tr ' ' '\n' > "$path"
    echo "$path"
}

# run_stats runs the built binary if present, otherwise falls back to go run
run_stats() {
    if [[ -x "./stats" ]]; then
        ./stats "$@"
    else
        go run . "$@"
    fi
}

FAILURES=0

echo "=============================================="
echo "Independent Verification of Stats Calculator"
echo "Using bc (arbitrary precision calculator)"
echo "Program output is read as JSON via jq"
echo "=============================================="
echo ""
echo "Dataset ($COUNT numbers):"
echo "$DATA" | tr ' ' '\n' | paste - - - - - - - - - -
echo ""

# Calculate sum
SUM=$(echo "$DATA" | tr ' ' '+' | bc -l)
echo "--- Basic Statistics ---"
printf "%-20s %s\n" "Count:" "$COUNT"
printf "%-20s %s\n" "Sum:" "$SUM"

# Calculate mean
MEAN=$(echo "scale=10; $SUM / $COUNT" | bc -l)
printf "%-20s %.10f\n" "Mean:" "$MEAN"

# Sort data and display
echo ""
echo "--- Sorted Data (for percentile verification) ---"
SORTED=$(echo "$DATA" | tr ' ' '\n' | sort -n | tr '\n' ' ')
echo "$SORTED"
echo ""

# Store sorted values in array for percentile calculations
SORTED_ARRAY=($(echo "$DATA" | tr ' ' '\n' | sort -n))

# Calculate variance and standard deviation
echo "--- Variance & Standard Deviation ---"

# Build the sum of squared deviations
SSQ_EXPR="scale=10; "
for val in $DATA; do
    SSQ_EXPR+="($val - $MEAN)^2 + "
done
SSQ_EXPR+="0"
SSQ=$(echo "$SSQ_EXPR" | bc -l)

VARIANCE=$(echo "scale=10; $SSQ / ($COUNT - 1)" | bc -l)
STDDEV=$(echo "scale=10; sqrt($VARIANCE)" | bc -l)

printf "%-20s %.10f\n" "Sum of Sq Dev:" "$SSQ"
printf "%-20s %.10f\n" "Variance (n-1):" "$VARIANCE"
printf "%-20s %.10f\n" "Std Deviation:" "$STDDEV"

# Coefficient of Variation
CV=$(echo "scale=10; ($STDDEV / $MEAN) * 100" | bc -l)
printf "%-20s %.10f%%\n" "CV:" "$CV"

# Standard Error of the Mean
STDERR=$(echo "scale=10; $STDDEV / sqrt($COUNT)" | bc -l)
printf "%-20s %.10f\n" "Std Error:" "$STDERR"

# 95% confidence interval for the mean (normal approximation)
CI_LOWER=$(echo "scale=10; $MEAN - 1.96 * $STDERR" | bc -l)
CI_UPPER=$(echo "scale=10; $MEAN + 1.96 * $STDERR" | bc -l)
printf "%-20s [%.10f, %.10f]\n" "95% CI (mean):" "$CI_LOWER" "$CI_UPPER"

# Percentile calculations
# Formula: rank = p * (n-1), then linear interpolation
echo ""
echo "--- Percentile Calculations ---"
echo "Formula: rank = p * (n-1), interpolate between floor and ceil indices"
echo ""

calculate_percentile() {
    local p=$1
    local rank
    rank=$(echo "scale=10; $p * ($COUNT - 1)" | bc -l)
    local lower_idx=${rank%%.*}
    # Handle case where rank is a whole number
    if [[ "$lower_idx" == "" ]]; then
        lower_idx=0
    fi
    local upper_idx=$((lower_idx + 1))
    if [[ $upper_idx -ge $COUNT ]]; then
        upper_idx=$((COUNT - 1))
    fi
    local weight
    weight=$(echo "scale=10; $rank - $lower_idx" | bc -l)
    local lower_val=${SORTED_ARRAY[$lower_idx]}
    local upper_val=${SORTED_ARRAY[$upper_idx]}
    echo "scale=10; $lower_val * (1 - $weight) + $upper_val * $weight" | bc -l
}

print_percentile_calc() {
    local p=$1
    local name=$2
    local rank
    rank=$(echo "scale=4; $p * ($COUNT - 1)" | bc -l)
    local lower_idx=${rank%%.*}
    if [[ "$lower_idx" == "" ]]; then
        lower_idx=0
    fi
    local upper_idx=$((lower_idx + 1))
    if [[ $upper_idx -ge $COUNT ]]; then
        upper_idx=$((COUNT - 1))
    fi
    local weight
    weight=$(echo "scale=2; $rank - $lower_idx" | bc -l)
    local lower_val=${SORTED_ARRAY[$lower_idx]}
    local upper_val=${SORTED_ARRAY[$upper_idx]}
    local result
    result=$(echo "scale=4; $lower_val * (1 - $weight) + $upper_val * $weight" | bc -l)
    printf "%-8s rank=%-5s idx[%2d]=%-6s idx[%2d]=%-6s weight=%-4s -> %s\n" \
        "$name" "$rank" "$lower_idx" "$lower_val" "$upper_idx" "$upper_val" "$weight" "$result"
}

print_percentile_calc 0.10 "P10"
print_percentile_calc 0.25 "Q1"
print_percentile_calc 0.50 "Median"
print_percentile_calc 0.75 "Q3"
print_percentile_calc 0.90 "P90"
print_percentile_calc 0.95 "P95"
print_percentile_calc 0.99 "P99"

P10=$(calculate_percentile 0.10)
Q1=$(calculate_percentile 0.25)
MEDIAN=$(calculate_percentile 0.50)
Q3=$(calculate_percentile 0.75)
P90=$(calculate_percentile 0.90)
P95=$(calculate_percentile 0.95)
P99=$(calculate_percentile 0.99)

# IQR
IQR=$(echo "scale=10; $Q3 - $Q1" | bc -l)
echo ""
printf "%-20s %s\n" "IQR (Q3 - Q1):" "$IQR"

# MAD (Median Absolute Deviation), scaled by the normal-consistency constant 1.4826
echo ""
echo "--- MAD (Median Absolute Deviation) ---"
DEV_SORTED=($(for val in "${SORTED_ARRAY[@]}"; do echo "scale=10; x = $val - $MEDIAN; if (x < 0) -x else x" | bc -l; done | sort -n))
# 31 deviations: the median is the element at 0-based index 15
MAD_RAW=${DEV_SORTED[15]}
MAD_SCALED=$(echo "scale=10; 1.4826 * $MAD_RAW" | bc -l)
printf "%-20s %s\n" "Raw MAD:" "$MAD_RAW"
printf "%-20s %.10f\n" "Scaled MAD:" "$MAD_SCALED"

# Min and Max
MIN=${SORTED_ARRAY[0]}
MAX=${SORTED_ARRAY[$((COUNT - 1))]}
echo ""
echo "--- Min/Max ---"
printf "%-20s %s\n" "Min:" "$MIN"
printf "%-20s %s\n" "Max:" "$MAX"

# Skewness
SKEW_EXPR="scale=10; "
for val in $DATA; do
    SKEW_EXPR+="(($val - $MEAN) / $STDDEV)^3 + "
done
SKEW_EXPR+="0"
SUM_CUBED=$(echo "$SKEW_EXPR" | bc -l)
SKEWNESS=$(echo "scale=10; ($COUNT / (($COUNT - 1) * ($COUNT - 2))) * $SUM_CUBED" | bc -l)
echo ""
echo "--- Skewness & Kurtosis ---"
printf "%-20s %.10f\n" "Skewness:" "$SKEWNESS"

# Kurtosis (excess kurtosis using sample formula)
KURT_EXPR="scale=10; "
for val in $DATA; do
    KURT_EXPR+="(($val - $MEAN) / $STDDEV)^4 + "
done
KURT_EXPR+="0"
SUM_FOURTH=$(echo "$KURT_EXPR" | bc -l)
KURTOSIS=$(echo "scale=10; ($COUNT * ($COUNT + 1)) / (($COUNT - 1) * ($COUNT - 2) * ($COUNT - 3)) * $SUM_FOURTH - 3 * ($COUNT - 1)^2 / (($COUNT - 2) * ($COUNT - 3))" | bc -l)
printf "%-20s %.10f\n" "Kurtosis:" "$KURTOSIS"

# Geometric Mean: exp(mean(ln x)), valid because all test values are positive
echo ""
echo "--- Geometric Mean, Distinct, Autocorrelation ---"
GEO_LOG_SUM="0"
for val in $DATA; do
    GEO_LOG_SUM=$(echo "scale=10; $GEO_LOG_SUM + l($val)" | bc -l)
done
GEOMEAN=$(echo "scale=10; e($GEO_LOG_SUM / $COUNT)" | bc -l)
printf "%-20s %.10f\n" "Geometric Mean:" "$GEOMEAN"

# Distinct count and duplicate percentage
DISTINCT=$(echo "$DATA" | tr ' ' '\n' | sort -n | uniq | wc -l | tr -d ' ')
DUP_PCT=$(echo "scale=10; ($COUNT - $DISTINCT) * 100 / $COUNT" | bc -l)
printf "%-20s %s\n" "Distinct:" "$DISTINCT"
printf "%-20s %.10f%%\n" "Duplicate Pct:" "$DUP_PCT"

# Lag-1 autocorrelation: sum((x_i - mean) * (x_i+1 - mean)) / sum((x_i - mean)^2)
# Numerator uses input order; denominator is the sum of squared deviations (SSQ)
AC_NUM_EXPR="scale=10; "
PREV=""
for val in $DATA; do
    if [[ -n "$PREV" ]]; then
        AC_NUM_EXPR+="($PREV - $MEAN) * ($val - $MEAN) + "
    fi
    PREV=$val
done
AC_NUM_EXPR+="0"
AC_NUM=$(echo "$AC_NUM_EXPR" | bc -l)
AUTOCORR=$(echo "scale=10; $AC_NUM / $SSQ" | bc -l)
printf "%-20s %.10f\n" "Autocorrelation:" "$AUTOCORR"

# Z-Score Outliers (threshold = 2.0)
echo ""
echo "--- Z-Score Outliers (threshold=2.0) ---"
Z_THRESHOLD="2.0"
Z_OUTLIER_COUNT=0
for val in $DATA; do
    Z=$(echo "scale=10; x=($val - $MEAN) / $STDDEV; if (x < 0) -x else x" | bc -l)
    IS_OUTLIER=$(echo "$Z > $Z_THRESHOLD" | bc -l)
    if [[ "$IS_OUTLIER" == "1" ]]; then
        printf "  %s has Z=%.4f > %s (OUTLIER)\n" "$val" "$Z" "$Z_THRESHOLD"
        Z_OUTLIER_COUNT=$((Z_OUTLIER_COUNT + 1))
    fi
done
if [[ $Z_OUTLIER_COUNT -eq 0 ]]; then
    echo "  No Z-score outliers found"
fi
printf "%-20s %d\n" "Z-Outlier Count:" "$Z_OUTLIER_COUNT"

# Modified Z-Score Outliers (Iglewicz-Hoaglin: 0.6745 * (x - median) / raw MAD)
echo ""
echo "--- Modified Z-Score Outliers (threshold=2.0) ---"
MODZ_OUTLIER_COUNT=0
for val in $DATA; do
    MODZ=$(echo "scale=10; x = 0.6745 * ($val - $MEDIAN) / $MAD_RAW; if (x < 0) -x else x" | bc -l)
    IS_OUTLIER=$(echo "$MODZ > $Z_THRESHOLD" | bc -l)
    if [[ "$IS_OUTLIER" == "1" ]]; then
        printf "  %s has modified Z=%.4f > %s (OUTLIER)\n" "$val" "$MODZ" "$Z_THRESHOLD"
        MODZ_OUTLIER_COUNT=$((MODZ_OUTLIER_COUNT + 1))
    fi
done
if [[ $MODZ_OUTLIER_COUNT -eq 0 ]]; then
    echo "  No modified z-score outliers found"
fi
printf "%-20s %d\n" "Mod Z-Outlier Count:" "$MODZ_OUTLIER_COUNT"

# IQR outliers at k=1.5 and k=3.0
echo ""
echo "--- IQR Outliers ---"
count_iqr_outliers() {
    local k=$1
    local lower upper n=0
    lower=$(echo "scale=10; $Q1 - $k * $IQR" | bc -l)
    upper=$(echo "scale=10; $Q3 + $k * $IQR" | bc -l)
    for val in $DATA; do
        if [[ "$(echo "$val < $lower || $val > $upper" | bc -l)" == "1" ]]; then
            n=$((n + 1))
        fi
    done
    echo "$n"
}
IQR_OUT_K15=$(count_iqr_outliers 1.5)
IQR_OUT_K30=$(count_iqr_outliers 3.0)
printf "%-20s %d\n" "Outliers (k=1.5):" "$IQR_OUT_K15"
printf "%-20s %d\n" "Outliers (k=3.0):" "$IQR_OUT_K30"

# EMA with span 5: alpha = 2/(5+1)
echo ""
echo "--- EMA (span 5) ---"
EMA_SPAN=5
EMA_ALPHA=$(echo "scale=10; 2 / ($EMA_SPAN + 1)" | bc -l)
EMA=""
for val in $DATA; do
    if [[ -z "$EMA" ]]; then
        EMA=$val
    else
        EMA=$(echo "scale=10; $EMA_ALPHA * $val + (1 - $EMA_ALPHA) * $EMA" | bc -l)
    fi
done
printf "%-20s %.10f\n" "EMA (span 5):" "$EMA"

# Now run the actual program and compare
echo ""
echo "=============================================="
echo "Running stats program on same dataset..."
echo "=============================================="
echo ""

TMPFILE=$(new_tempfile "$DATA")
JSON=$(run_stats -j -z 2.0 -p "10,90" "$TMPFILE")

# jq always exits 0 for a missing key (it yields null), so no comparison below
# can abort the script before its result is reported.
jget() {
    echo "$JSON" | jq -r "$1"
}

run_stats -z 2.0 "$TMPFILE"
echo ""

# Comparison helpers
compare_values() {
    local name=$1
    local bc_val=$2
    local prog_val=$3
    local tolerance=0.0001

    if [[ -z "$prog_val" || "$prog_val" == "null" ]]; then
        printf "| %-14s | %15s | %15s | %-6s |\n" "$name" "$bc_val" "MISSING" "x"
        FAILURES=$((FAILURES + 1))
        return
    fi

    local diff match
    diff=$(echo "scale=10; x=$bc_val - $prog_val; if (x < 0) -x else x" | bc -l)
    match=$(echo "$diff < $tolerance" | bc -l)

    if [[ "$match" == "1" ]]; then
        printf "| %-14s | %15.4f | %15.4f | %-6s |\n" "$name" "$bc_val" "$prog_val" "ok"
    else
        printf "| %-14s | %15.4f | %15.4f | %-6s |\n" "$name" "$bc_val" "$prog_val" "x"
        FAILURES=$((FAILURES + 1))
    fi
}

check() {
    local description=$1
    local actual=$2
    local expected=$3
    if [[ "$actual" == "$expected" ]]; then
        echo "PASS: $description"
    else
        echo "FAIL: $description (got '$actual', expected '$expected')"
        FAILURES=$((FAILURES + 1))
    fi
}

echo "=============================================="
echo "Verification Summary"
echo "=============================================="
echo ""
printf "| %-14s | %15s | %15s | %-6s |\n" "Statistic" "bc Calculation" "Program Output" "Match"
printf "|----------------|-----------------|-----------------|--------|\n"
compare_values "Count"          "$COUNT"       "$(jget .count)"
compare_values "Sum"            "$SUM"         "$(jget .sum)"
compare_values "Mean"           "$MEAN"        "$(jget .mean)"
compare_values "Variance"       "$VARIANCE"    "$(jget .variance)"
compare_values "StdDev"         "$STDDEV"      "$(jget .stdDev)"
compare_values "Min"            "$MIN"         "$(jget .min)"
compare_values "Max"            "$MAX"         "$(jget .max)"
compare_values "Q1 (p25)"       "$Q1"          "$(jget .q1)"
compare_values "Median (p50)"   "$MEDIAN"      "$(jget .median)"
compare_values "Q3 (p75)"       "$Q3"          "$(jget .q3)"
compare_values "P95"            "$P95"         "$(jget .p95)"
compare_values "P99"            "$P99"         "$(jget .p99)"
compare_values "IQR"            "$IQR"         "$(jget .iqr)"
compare_values "CV (%)"         "$CV"          "$(jget .cv)"
compare_values "StdErr"         "$STDERR"      "$(jget .stdErr)"
compare_values "CI95 Lower"     "$CI_LOWER"    "$(jget .ci95Lower)"
compare_values "CI95 Upper"     "$CI_UPPER"    "$(jget .ci95Upper)"
compare_values "GeoMean"        "$GEOMEAN"     "$(jget .geometricMean)"
compare_values "MAD"            "$MAD_SCALED"  "$(jget .mad)"
compare_values "Distinct"       "$DISTINCT"    "$(jget .distinct)"
compare_values "Dup Pct (%)"    "$DUP_PCT"     "$(jget .duplicatePct)"
compare_values "Autocorr"       "$AUTOCORR"    "$(jget .autocorrelation)"
compare_values "Skewness"       "$SKEWNESS"    "$(jget .skewness)"
compare_values "Kurtosis"       "$KURTOSIS"    "$(jget .kurtosis)"
compare_values "Z-Outliers"     "$Z_OUTLIER_COUNT"    "$(jget '.zScoreOutliers | length')"
compare_values "Mod Z-Out"      "$MODZ_OUTLIER_COUNT" "$(jget '.modZOutliers | length')"
compare_values "IQR Outliers"   "$IQR_OUT_K15"        "$(jget '.outliers | length')"
compare_values "Custom p10"     "$P10"         "$(jget '.customPercentiles["10"]')"
compare_values "Custom p90"     "$P90"         "$(jget '.customPercentiles["90"]')"
echo ""

# --- Structural checks on the JSON payload ---
echo "--- Structural Checks ---"
check "isSymmetric is false for the asymmetric dataset" "$(jget .isSymmetric)" "false"
check "inputOrder reports unordered"                    "$(jget .inputOrder)" "unordered"
check "skippedLines is 0 for clean input"               "$(jget .skippedLines)" "0"
check "zScoreValid is true"                             "$(jget .zScoreValid)" "true"
check "modZValid is true"                               "$(jget .modZValid)" "true"
check "modalBinValid is false when a mode exists"       "$(jget .modalBinValid)" "false"
check "mode is [50]"                                    "$(jget '.mode | join(",")')" "50"
check "histogram has 16 bins"                           "$(jget '.histogram | length')" "16"
check "trendline has 16 bins"                           "$(jget '.trendline | length')" "16"
check "version matches the binary"                      "$(jget .version)" "$(run_stats -v | head -1 | awk '{print $NF}')"
echo ""

# --- Outlier sensitivity (-k) ---
echo "=============================================="
echo "Outlier Sensitivity Verification (-k)"
echo "=============================================="
echo ""
K30_JSON=$(run_stats -j -k 3.0 "$TMPFILE")
check "k=3.0 flags $IQR_OUT_K30 outlier(s)" "$(echo "$K30_JSON" | jq -r '.outliers | length')" "$IQR_OUT_K30"
check "k=3.0 is recorded in the output"     "$(echo "$K30_JSON" | jq -r '.iqrMultiplier')" "3"

set +e
K_BAD_OUTPUT=$(run_stats -k 0 "$TMPFILE" 2>&1)
K_BAD_STATUS=$?
set -e
if [[ $K_BAD_STATUS -ne 0 ]] && [[ "$K_BAD_OUTPUT" == *"greater than 0"* ]]; then
    echo "PASS: -k 0 is rejected"
else
    echo "FAIL: expected -k 0 to be rejected (exit=$K_BAD_STATUS)"
    FAILURES=$((FAILURES + 1))
fi
echo ""

# --- EMA Verification ---
echo "=============================================="
echo "EMA Verification (-e 5)"
echo "=============================================="
echo ""
EMA_JSON=$(run_stats -j -e 5 "$TMPFILE")
printf "| %-14s | %15s | %15s | %-6s |\n" "Statistic" "bc Calculation" "Program Output" "Match"
printf "|----------------|-----------------|-----------------|--------|\n"
compare_values "EMA (span 5)" "$EMA" "$(echo "$EMA_JSON" | jq -r '.ema')"
check "emaSpan is recorded" "$(echo "$EMA_JSON" | jq -r '.emaSpan')" "5"
echo ""

# --- Trimmed Mean Verification ---
echo "=============================================="
echo "Trimmed Mean Verification (-t 10)"
echo "=============================================="
echo ""

# trimCount = floor(31 * 10 / 100) = 3
# Remove 3 from each end of sorted data, average remaining 25 values
TRIM_COUNT=3
TRIM_REMAINING=$((COUNT - 2 * TRIM_COUNT))

TRIM_SUM="0"
for i in $(seq $TRIM_COUNT $((COUNT - TRIM_COUNT - 1))); do
    TRIM_SUM=$(echo "scale=10; $TRIM_SUM + ${SORTED_ARRAY[$i]}" | bc -l)
done
TRIM_MEAN=$(echo "scale=10; $TRIM_SUM / $TRIM_REMAINING" | bc -l)
printf "%-20s %s (from %d values, trimmed %d from each end)\n" "Trimmed Mean:" "$TRIM_MEAN" "$TRIM_REMAINING" "$TRIM_COUNT"
echo ""

TRIM_JSON=$(run_stats -j -t 10 "$TMPFILE")
printf "| %-14s | %15s | %15s | %-6s |\n" "Statistic" "bc Calculation" "Program Output" "Match"
printf "|----------------|-----------------|-----------------|--------|\n"
compare_values "Trim Mean" "$TRIM_MEAN" "$(echo "$TRIM_JSON" | jq -r '.trimmedMean')"
compare_values "Full Mean"  "$MEAN"      "$(echo "$TRIM_JSON" | jq -r '.mean')"
echo ""

# --- Log Transform Verification ---
echo "=============================================="
echo "Log Transform Verification (-l)"
echo "=============================================="
echo ""

LOG_SUM="0"
for val in $DATA; do
    LOG_SUM=$(echo "scale=10; $LOG_SUM + l($val)" | bc -l)
done
LOG_MEAN=$(echo "scale=10; $LOG_SUM / $COUNT" | bc -l)

LOG_SSQ="0"
for val in $DATA; do
    LOG_VAL=$(echo "scale=10; l($val)" | bc -l)
    LOG_SSQ=$(echo "scale=10; $LOG_SSQ + ($LOG_VAL - $LOG_MEAN)^2" | bc -l)
done
LOG_VARIANCE=$(echo "scale=10; $LOG_SSQ / ($COUNT - 1)" | bc -l)
LOG_STDDEV=$(echo "scale=10; sqrt($LOG_VARIANCE)" | bc -l)

LOG_JSON=$(run_stats -j -l "$TMPFILE")
printf "| %-14s | %15s | %15s | %-6s |\n" "Statistic" "bc Calculation" "Program Output" "Match"
printf "|----------------|-----------------|-----------------|--------|\n"
compare_values "Log Mean"     "$LOG_MEAN"     "$(echo "$LOG_JSON" | jq -r '.mean')"
compare_values "Log StdDev"   "$LOG_STDDEV"   "$(echo "$LOG_JSON" | jq -r '.stdDev')"
compare_values "Log Variance" "$LOG_VARIANCE" "$(echo "$LOG_JSON" | jq -r '.variance')"
check "geoMeanValid is false under -l" "$(echo "$LOG_JSON" | jq -r '.geoMeanValid')" "false"
check "logTransformed is true"         "$(echo "$LOG_JSON" | jq -r '.logTransformed')" "true"
echo ""

# --- Symmetry Verification ---
echo "=============================================="
echo "Symmetry Verification"
echo "=============================================="
echo ""

# Same 40-value fixture as symmetricTestData in stats_test.go:
# 20 disjoint pairs, each summing to 1000, symmetric about 500
SYMDATA="612 137 827 495 958 264 709 19 882 455 349 736 88 981 545 233 61 694 42 767 388 118 912 651 306 994 173 588 505 377 863 220 939 6 623 780 151 849 291 412"
SYM_COUNT=40
SYM_SORTED=($(echo "$SYMDATA" | tr ' ' '\n' | sort -n))

# Independently verify every sorted pair sums to 2 * center, pairing from both ends inward
SYM_CENTER=$(echo "scale=10; (${SYM_SORTED[0]} + ${SYM_SORTED[$((SYM_COUNT - 1))]}) / 2" | bc -l)
PAIR_FAILURES=0
for i in $(seq 0 $((SYM_COUNT / 2 - 1))); do
    PAIR_SUM=$(echo "${SYM_SORTED[$i]} + ${SYM_SORTED[$((SYM_COUNT - 1 - i))]}" | bc -l)
    MATCH=$(echo "$PAIR_SUM == 2 * $SYM_CENTER" | bc -l)
    if [[ "$MATCH" != "1" ]]; then
        echo "FAIL: pair ${SYM_SORTED[$i]} + ${SYM_SORTED[$((SYM_COUNT - 1 - i))]} = $PAIR_SUM"
        PAIR_FAILURES=$((PAIR_FAILURES + 1))
    fi
done
if [[ $PAIR_FAILURES -eq 0 ]]; then
    printf "PASS: all %d sorted pairs sum to 2 * center (center=%.0f)\n" $((SYM_COUNT / 2)) "$SYM_CENTER"
else
    FAILURES=$((FAILURES + PAIR_FAILURES))
fi

SYMFILE=$(new_tempfile "$SYMDATA")
SYM_JSON=$(run_stats -j "$SYMFILE")
check "isSymmetric is true"      "$(echo "$SYM_JSON" | jq -r '.isSymmetric')"    "true"
check "symmetryCenter is 500"    "$(echo "$SYM_JSON" | jq -r '.symmetryCenter')" "500"
check "symmetryPairs is 20"      "$(echo "$SYM_JSON" | jq -r '.symmetryPairs')"  "20"
# All 40 values are distinct, so the mode falls back to the densest histogram bin
check "mode is empty"            "$(echo "$SYM_JSON" | jq -r '.mode | length')"  "0"
check "modalBinValid is true"    "$(echo "$SYM_JSON" | jq -r '.modalBinValid')"  "true"
echo ""

# --- Skipped Lines Verification ---
echo "=============================================="
echo "Skipped Lines Verification"
echo "=============================================="
echo ""

# 2 unparseable lines (abc, xyz) plus 2 non-finite values that parse but must be
# rejected (NaN, Inf); the 2 blank lines must not be counted
SKIPFILE=$(mktemp)
TMPFILES+=("$SKIPFILE")
printf '10\n\nabc\n20\n\nxyz\n30\nNaN\nInf\n' > "$SKIPFILE"
SKIP_JSON=$(run_stats -j "$SKIPFILE" 2>/dev/null)
check "skippedLines is 4 (blank lines excluded)" "$(echo "$SKIP_JSON" | jq -r '.skippedLines')" "4"
check "count is 3 valid values"                  "$(echo "$SKIP_JSON" | jq -r '.count')" "3"
check "sum is unaffected by NaN"                 "$(echo "$SKIP_JSON" | jq -r '.sum')" "60"
check "max is unaffected by Inf"                 "$(echo "$SKIP_JSON" | jq -r '.max')" "30"
echo ""

# --- Input Order Verification ---
echo "=============================================="
echo "Input Order Verification (sorted input)"
echo "=============================================="
echo ""

SORTFILE=$(new_tempfile "$SORTED")
check "sorted input reports ascending" "$(run_stats -j "$SORTFILE" | jq -r '.inputOrder')" "ascending"

ORDER_TEXT=$(run_stats "$SORTFILE")
if [[ "$ORDER_TEXT" == *"WARNING: trendline and autocorrelation reflect this sort order"* ]]; then
    echo "PASS: sorted input carries the sort-order warning"
else
    echo "FAIL: sorted input is missing the sort-order warning"
    FAILURES=$((FAILURES + 1))
fi
ORDER_EMA_TEXT=$(run_stats -e 5 "$SORTFILE")
if [[ "$ORDER_EMA_TEXT" == *"WARNING: trendline, autocorrelation, and EMA reflect this sort order"* ]]; then
    echo "PASS: sort-order warning names EMA when -e is active"
else
    echo "FAIL: sort-order warning does not name EMA under -e"
    FAILURES=$((FAILURES + 1))
fi
echo ""

# --- Degenerate Z-Score Verification ---
echo "=============================================="
echo "Degenerate Z-Score Verification (zero spread)"
echo "=============================================="
echo ""

FLATFILE=$(new_tempfile "5 5 5 5")
FLAT_JSON=$(run_stats -j -z 2.0 "$FLATFILE")
check "zScoreThreshold is still recorded" "$(echo "$FLAT_JSON" | jq -r '.zScoreThreshold')" "2"
check "zScoreValid is false"              "$(echo "$FLAT_JSON" | jq -r '.zScoreValid')" "false"
check "modZValid is false"                "$(echo "$FLAT_JSON" | jq -r '.modZValid')" "false"

FLAT_TEXT=$(run_stats -z 2.0 "$FLATFILE")
if [[ "$FLAT_TEXT" == *"N/A - standard deviation is zero"* ]]; then
    echo "PASS: Z-outlier line reports N/A rather than vanishing"
else
    echo "FAIL: Z-outlier line is missing for zero-spread data"
    FAILURES=$((FAILURES + 1))
fi
echo ""

# --- Small Magnitude Formatting Verification ---
echo "=============================================="
echo "Small Magnitude Formatting Verification"
echo "=============================================="
echo ""

TINYFILE=$(new_tempfile "0.00004 0.00006")
TINY_TEXT=$(run_stats "$TINYFILE")
for expected in "0.00004" "0.00006" "0.00005"; do
    if [[ "$TINY_TEXT" == *"$expected"* ]]; then
        echo "PASS: $expected survives formatting"
    else
        echo "FAIL: $expected was lost in formatting"
        FAILURES=$((FAILURES + 1))
    fi
done
if [[ "$TINY_TEXT" == *"e-0"* ]] || [[ "$TINY_TEXT" == *"E-0"* ]]; then
    echo "FAIL: output used scientific notation"
    FAILURES=$((FAILURES + 1))
else
    echo "PASS: output avoids scientific notation"
fi
echo ""

# --- Trim Dataset (-T) Verification ---
echo "=============================================="
echo "Trim Dataset Verification (-T 10)"
echo "=============================================="
echo ""

TRIMD_JSON=$(run_stats -j -T 10 "$TMPFILE")
check "count is 25 (trimmed from 31)"  "$(echo "$TRIMD_JSON" | jq -r '.count')" "25"
check "trimDatasetOrigN is 31"         "$(echo "$TRIMD_JSON" | jq -r '.trimDatasetOrigN')" "31"
check "trimDatasetPct is 10"           "$(echo "$TRIMD_JSON" | jq -r '.trimDatasetPct')" "10"
check "trendline is suppressed"        "$(echo "$TRIMD_JSON" | jq -r '.trendline')" ""
check "autocorrValid is false"         "$(echo "$TRIMD_JSON" | jq -r '.autocorrValid')" "false"
check "inputOrder is suppressed"       "$(echo "$TRIMD_JSON" | jq -r '.inputOrder')" ""
compare_values "Trimmed Mean" "$TRIM_MEAN" "$(echo "$TRIMD_JSON" | jq -r '.mean')"

TRIMD_TEXT=$(run_stats -T 10 "$TMPFILE")
if [[ "$TRIMD_TEXT" == *"(trimmed dataset: 10% from each tail, 31 → 25 values)"* ]]; then
    echo "PASS: trimmed-dataset header present"
else
    echo "FAIL: trimmed-dataset header missing or malformed"
    FAILURES=$((FAILURES + 1))
fi
if [[ "$TRIMD_TEXT" == *"all statistics above are computed on the trimmed dataset"* ]]; then
    echo "PASS: trimmed-dataset footnote present"
else
    echo "FAIL: trimmed-dataset footnote missing"
    FAILURES=$((FAILURES + 1))
fi
# The star convention was dropped: no label may end in '*'
if echo "$TRIMD_TEXT" | grep -qE '^[A-Za-z0-9 ()%>.]+\*:'; then
    echo "FAIL: found a starred label; the star convention was removed"
    FAILURES=$((FAILURES + 1))
else
    echo "PASS: no starred labels under -T"
fi
echo ""

# --- Mutual Exclusion Verification ---
echo "=============================================="
echo "Mutual Exclusion Verification"
echo "=============================================="
echo ""

check_rejected() {
    local description=$1
    shift
    local output status
    set +e
    output=$(run_stats "$@" 2>&1)
    status=$?
    set -e
    if [[ $status -ne 0 ]] && [[ "$output" == *"$EXPECT_SUBSTR"* ]]; then
        echo "PASS: $description"
    else
        echo "FAIL: $description (exit=$status)"
        FAILURES=$((FAILURES + 1))
    fi
}

EXPECT_SUBSTR="mutually exclusive"
check_rejected "-t and -T are mutually exclusive" -t 10 -T 10 "$TMPFILE"
check_rejected "-e and -T are mutually exclusive" -e 5 -T 10 "$TMPFILE"

EXPECT_SUBSTR="expected at most one input file"
check_rejected "options after the filename are rejected" "$TMPFILE" -z 2.0
check_rejected "two input files are rejected" "$TMPFILE" "$TMPFILE"

echo ""

if [[ $FAILURES -eq 0 ]]; then
    echo "Verification complete. All values match."
    exit 0
else
    echo "Verification FAILED. $FAILURES check(s) did not match."
    exit 1
fi
