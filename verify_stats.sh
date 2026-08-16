#!/bin/bash
# verify_stats.sh - Independent verification of stats calculations using bc
# Uses the same 31-number dataset from stats_test.go
# Created on MacOS Sequoia 15.7.3
# COMPATIBILITY: Must work with bash 5.2+ (GitHub Actions requirement)

set -e

# Dataset (same as testData in stats_test.go)
DATA="5 10 15.5 20 25 30 35 40 45 50 55 60 65 70 75.25 80 85 90 95 100 12.5 37.5 62.5 87.5 50 50 50 3 150 7.75 42"
COUNT=31

# run_stats runs the built binary if present, otherwise falls back to go run
run_stats() {
    if [[ -f "./stats" ]]; then
        ./stats "$@"
    else
        go run stats.go "$@"
    fi
}

echo "=============================================="
echo "Independent Verification of Stats Calculator"
echo "Using bc (arbitrary precision calculator)"
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
VARIANCE_CALC=$(cat << EOF | bc -l
scale=10
mean = $MEAN
ssq = 0
EOF
)

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

# Percentile calculations
# Formula: rank = p * (n-1), then linear interpolation
echo ""
echo "--- Percentile Calculations ---"
echo "Formula: rank = p * (n-1), interpolate between floor and ceil indices"
echo ""

calculate_percentile() {
    local p=$1
    local rank=$(echo "scale=10; $p * ($COUNT - 1)" | bc -l)
    local lower_idx=$(echo "$rank" | cut -d'.' -f1)
    # Handle case where rank is a whole number
    if [[ "$lower_idx" == "" ]]; then
        lower_idx=0
    fi
    local upper_idx=$((lower_idx + 1))
    if [[ $upper_idx -ge $COUNT ]]; then
        upper_idx=$((COUNT - 1))
    fi
    local weight=$(echo "scale=10; $rank - $lower_idx" | bc -l)
    local lower_val=${SORTED_ARRAY[$lower_idx]}
    local upper_val=${SORTED_ARRAY[$upper_idx]}
    local result=$(echo "scale=10; $lower_val * (1 - $weight) + $upper_val * $weight" | bc -l)
    echo "$result"
}

print_percentile_calc() {
    local p=$1
    local name=$2
    local rank=$(echo "scale=4; $p * ($COUNT - 1)" | bc -l)
    local lower_idx=$(echo "$rank" | cut -d'.' -f1)
    if [[ "$lower_idx" == "" ]]; then
        lower_idx=0
    fi
    local upper_idx=$((lower_idx + 1))
    if [[ $upper_idx -ge $COUNT ]]; then
        upper_idx=$((COUNT - 1))
    fi
    local weight=$(echo "scale=2; $rank - $lower_idx" | bc -l)
    local lower_val=${SORTED_ARRAY[$lower_idx]}
    local upper_val=${SORTED_ARRAY[$upper_idx]}
    local result=$(echo "scale=4; $lower_val * (1 - $weight) + $upper_val * $weight" | bc -l)
    printf "%-8s rank=%-5s idx[%2d]=%-6s idx[%2d]=%-6s weight=%-4s -> %s\n" \
        "$name" "$rank" "$lower_idx" "$lower_val" "$upper_idx" "$upper_val" "$weight" "$result"
}

print_percentile_calc 0.25 "Q1"
print_percentile_calc 0.50 "Median"
print_percentile_calc 0.75 "Q3"
print_percentile_calc 0.95 "P95"
print_percentile_calc 0.99 "P99"

Q1=$(calculate_percentile 0.25)
MEDIAN=$(calculate_percentile 0.50)
Q3=$(calculate_percentile 0.75)
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
Z_OUTLIERS=""
Z_OUTLIER_COUNT=0
for val in $DATA; do
    Z=$(echo "scale=10; x=($val - $MEAN) / $STDDEV; if (x < 0) -x else x" | bc -l)
    IS_OUTLIER=$(echo "$Z > $Z_THRESHOLD" | bc -l)
    if [[ "$IS_OUTLIER" == "1" ]]; then
        printf "  %s has Z=%.4f > %s (OUTLIER)\n" "$val" "$Z" "$Z_THRESHOLD"
        Z_OUTLIERS="$Z_OUTLIERS $val"
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

# Now run the actual program and compare
echo ""
echo "=============================================="
echo "Running stats program on same dataset..."
echo "=============================================="
echo ""

# Create temp file with dataset
TMPFILE=$(mktemp)
echo "$DATA" | tr ' ' '\n' > "$TMPFILE"

# Run stats program (assuming it's built or we use go run)
if [[ -f "./stats" ]]; then
    PROGRAM_OUTPUT=$(./stats -z 2.0 "$TMPFILE")
elif command -v go &> /dev/null; then
    PROGRAM_OUTPUT=$(go run stats.go -z 2.0 "$TMPFILE")
else
    echo "Error: Neither ./stats binary nor go command found"
    rm "$TMPFILE"
    exit 1
fi

rm "$TMPFILE"

echo "$PROGRAM_OUTPUT"
echo ""

# Extract values from program output for comparison
extract_value() {
    echo "$PROGRAM_OUTPUT" | grep -E "^$1" | awk '{print $NF}' | tr -d '()'
}

PROG_SUM=$(extract_value "Sum:")
PROG_MEAN=$(extract_value "Mean:")
PROG_MEDIAN=$(extract_value "Median")
PROG_STDDEV=$(extract_value "Std Deviation:")
PROG_VARIANCE=$(extract_value "Variance:")
PROG_Q1=$(extract_value "Quartile 1")
PROG_Q3=$(extract_value "Quartile 3")
PROG_P95=$(echo "$PROGRAM_OUTPUT" | grep "p95" | awk '{print $NF}')
PROG_P99=$(echo "$PROGRAM_OUTPUT" | grep "p99" | awk '{print $NF}')
PROG_IQR=$(extract_value "IQR:")
PROG_CV=$(echo "$PROGRAM_OUTPUT" | grep "^CV:" | awk '{print $2}' | tr -d '%')
PROG_SKEWNESS=$(echo "$PROGRAM_OUTPUT" | grep "^Skewness:" | awk '{print $2}')
PROG_KURTOSIS=$(echo "$PROGRAM_OUTPUT" | grep "^Kurtosis:" | awk '{print $2}')
PROG_MIN=$(extract_value "Min:")
PROG_MAX=$(extract_value "Max:")
PROG_STDERR=$(extract_value "Std Error:")
PROG_GEOMEAN=$(extract_value "Geometric Mean:")
PROG_MAD=$(extract_value "MAD:")
PROG_DISTINCT=$(echo "$PROGRAM_OUTPUT" | grep "^Distinct:" | awk '{print $2}')
PROG_DUP_PCT=$(echo "$PROGRAM_OUTPUT" | grep "^Distinct:" | awk '{print $5}' | tr -d '(%')
PROG_AUTOCORR=$(echo "$PROGRAM_OUTPUT" | grep "^Autocorrelation:" | awk '{print $2}')

# Comparison function (using bc for float comparison)
FAILURES=0

compare_values() {
    local name=$1
    local bc_val=$2
    local prog_val=$3
    local tolerance=0.0001

    # Handle empty values
    if [[ -z "$prog_val" ]]; then
        printf "| %-12s | %15s | %15s | %-6s |\n" "$name" "$bc_val" "N/A" "SKIP"
        FAILURES=$((FAILURES + 1))
        return
    fi

    local diff=$(echo "scale=10; x=$bc_val - $prog_val; if (x < 0) -x else x" | bc -l)
    local match=$(echo "$diff < $tolerance" | bc -l)

    if [[ "$match" == "1" ]]; then
        printf "| %-12s | %15.4f | %15s | %-6s |\n" "$name" "$bc_val" "$prog_val" "✓"
    else
        printf "| %-12s | %15.4f | %15s | %-6s |\n" "$name" "$bc_val" "$prog_val" "✗"
        FAILURES=$((FAILURES + 1))
    fi
}

echo "=============================================="
echo "Verification Summary"
echo "=============================================="
echo ""
printf "| %-12s | %15s | %15s | %-6s |\n" "Statistic" "bc Calculation" "Program Output" "Match"
printf "|--------------|-----------------|-----------------|--------|\n"
compare_values "Sum" "$SUM" "$PROG_SUM"
compare_values "Mean" "$MEAN" "$PROG_MEAN"
compare_values "Variance" "$VARIANCE" "$PROG_VARIANCE"
compare_values "StdDev" "$STDDEV" "$PROG_STDDEV"
compare_values "Min" "$MIN" "$PROG_MIN"
compare_values "Max" "$MAX" "$PROG_MAX"
compare_values "Q1 (p25)" "$Q1" "$PROG_Q1"
compare_values "Median (p50)" "$MEDIAN" "$PROG_MEDIAN"
compare_values "Q3 (p75)" "$Q3" "$PROG_Q3"
compare_values "P95" "$P95" "$PROG_P95"
compare_values "P99" "$P99" "$PROG_P99"
compare_values "IQR" "$IQR" "$PROG_IQR"
compare_values "CV (%)" "$CV" "$PROG_CV"
compare_values "StdErr" "$STDERR" "$PROG_STDERR"
compare_values "GeoMean" "$GEOMEAN" "$PROG_GEOMEAN"
compare_values "MAD" "$MAD_SCALED" "$PROG_MAD"
compare_values "Distinct" "$DISTINCT" "$PROG_DISTINCT"
compare_values "Dup Pct (%)" "$DUP_PCT" "$PROG_DUP_PCT"
compare_values "Autocorr" "$AUTOCORR" "$PROG_AUTOCORR"
compare_values "Skewness" "$SKEWNESS" "$PROG_SKEWNESS"
compare_values "Kurtosis" "$KURTOSIS" "$PROG_KURTOSIS"

# Extract Z-score outlier count from program output
PROG_Z_LINE=$(echo "$PROGRAM_OUTPUT" | grep "^Z-Outliers")
if [[ -n "$PROG_Z_LINE" ]]; then
    # Count values in brackets: extract bracket content, count space-separated items
    PROG_Z_CONTENT=$(echo "$PROG_Z_LINE" | sed 's/.*\[//' | sed 's/\].*//')
    if [[ "$PROG_Z_CONTENT" == *"None"* ]] || [[ -z "$PROG_Z_CONTENT" ]]; then
        PROG_Z_COUNT=0
    else
        PROG_Z_COUNT=$(echo "$PROG_Z_CONTENT" | wc -w | tr -d ' ')
    fi
    compare_values "Z-Outliers" "$Z_OUTLIER_COUNT" "$PROG_Z_COUNT"
else
    printf "| %-12s | %15s | %15s | %-6s |\n" "Z-Outliers" "$Z_OUTLIER_COUNT" "N/A" "SKIP"
    FAILURES=$((FAILURES + 1))
fi

# Extract modified z-score outlier count from program output
PROG_MODZ_LINE=$(echo "$PROGRAM_OUTPUT" | grep "^Mod Z-Outliers" || true)
if [[ -n "$PROG_MODZ_LINE" ]]; then
    PROG_MODZ_CONTENT=$(echo "$PROG_MODZ_LINE" | sed 's/.*\[//' | sed 's/\].*//')
    if [[ "$PROG_MODZ_CONTENT" == *"None"* ]] || [[ -z "$PROG_MODZ_CONTENT" ]]; then
        PROG_MODZ_COUNT=0
    else
        PROG_MODZ_COUNT=$(echo "$PROG_MODZ_CONTENT" | wc -w | tr -d ' ')
    fi
    compare_values "Mod Z-Out" "$MODZ_OUTLIER_COUNT" "$PROG_MODZ_COUNT"
else
    printf "| %-12s | %15s | %15s | %-6s |\n" "Mod Z-Out" "$MODZ_OUTLIER_COUNT" "N/A" "SKIP"
    FAILURES=$((FAILURES + 1))
fi
echo ""

# --- Structural checks on the main output (v1.12.0 / v1.13.0 lines) ---
echo "--- Structural Checks (main output) ---"
if echo "$PROGRAM_OUTPUT" | grep -q "^Symmetry:.*None"; then
    echo "PASS: Symmetry reports None for the asymmetric dataset"
else
    echo "FAIL: expected 'Symmetry: None'"
    FAILURES=$((FAILURES + 1))
fi
if echo "$PROGRAM_OUTPUT" | grep -q "^Input Order:.*unordered"; then
    echo "PASS: Input Order reports unordered"
else
    echo "FAIL: expected 'Input Order: unordered'"
    FAILURES=$((FAILURES + 1))
fi
if echo "$PROGRAM_OUTPUT" | grep -q "^Skipped:"; then
    echo "FAIL: Skipped line should be absent for clean input"
    FAILURES=$((FAILURES + 1))
else
    echo "PASS: no Skipped line for clean input"
fi
echo ""

# --- Trimmed Mean Verification ---
echo "=============================================="
echo "Trimmed Mean Verification (trim=10%)"
echo "=============================================="
echo ""

# trimCount = floor(31 * 10 / 100) = 3
# Remove 3 from each end of sorted data, average remaining 25 values
# Sorted indices 3..27 (0-based)
TRIM_COUNT=3
TRIM_REMAINING=$((COUNT - 2 * TRIM_COUNT))

TRIM_SUM="0"
for i in $(seq $TRIM_COUNT $((COUNT - TRIM_COUNT - 1))); do
    TRIM_SUM=$(echo "scale=10; $TRIM_SUM + ${SORTED_ARRAY[$i]}" | bc -l)
done
TRIM_MEAN=$(echo "scale=10; $TRIM_SUM / $TRIM_REMAINING" | bc -l)
printf "%-20s %s (from %d values, trimmed %d from each end)\n" "Trimmed Mean:" "$TRIM_MEAN" "$TRIM_REMAINING" "$TRIM_COUNT"

# Run program with -t 10
TMPFILE3=$(mktemp)
echo "$DATA" | tr ' ' '\n' > "$TMPFILE3"

if [[ -f "./stats" ]]; then
    TRIM_OUTPUT=$(./stats -t 10 "$TMPFILE3")
elif command -v go &> /dev/null; then
    TRIM_OUTPUT=$(go run stats.go -t 10 "$TMPFILE3")
else
    echo "Error: Neither ./stats binary nor go command found"
    rm "$TMPFILE3"
    exit 1
fi

rm "$TMPFILE3"

PROG_TRIM_MEAN=$(echo "$TRIM_OUTPUT" | grep "^Trimmed Mean" | awk '{print $NF}')

echo ""
printf "| %-12s | %15s | %15s | %-6s |\n" "Statistic" "bc Calculation" "Program Output" "Match"
printf "|--------------|-----------------|-----------------|--------|\n"
compare_values "Trim Mean" "$TRIM_MEAN" "$PROG_TRIM_MEAN"
echo ""

# --- Log Transform Verification ---
echo "=============================================="
echo "Log Transform Verification"
echo "=============================================="
echo ""

# Compute ln of each value and their mean using bc
LOG_SUM="0"
for val in $DATA; do
    LOG_SUM=$(echo "scale=10; $LOG_SUM + l($val)" | bc -l)
done
LOG_MEAN=$(echo "scale=10; $LOG_SUM / $COUNT" | bc -l)

# Compute ln variance and stddev
LOG_SSQ="0"
for val in $DATA; do
    LOG_VAL=$(echo "scale=10; l($val)" | bc -l)
    LOG_SSQ=$(echo "scale=10; $LOG_SSQ + ($LOG_VAL - $LOG_MEAN)^2" | bc -l)
done
LOG_VARIANCE=$(echo "scale=10; $LOG_SSQ / ($COUNT - 1)" | bc -l)
LOG_STDDEV=$(echo "scale=10; sqrt($LOG_VARIANCE)" | bc -l)

# Run program with -l flag
TMPFILE2=$(mktemp)
echo "$DATA" | tr ' ' '\n' > "$TMPFILE2"

if [[ -f "./stats" ]]; then
    LOG_OUTPUT=$(./stats -l "$TMPFILE2")
elif command -v go &> /dev/null; then
    LOG_OUTPUT=$(go run stats.go -l "$TMPFILE2")
else
    echo "Error: Neither ./stats binary nor go command found"
    rm "$TMPFILE2"
    exit 1
fi

rm "$TMPFILE2"

echo "$LOG_OUTPUT"
echo ""

# Extract values from log-transformed output
PROG_LOG_MEAN=$(echo "$LOG_OUTPUT" | grep "^Mean:" | awk '{print $2}')
PROG_LOG_STDDEV=$(echo "$LOG_OUTPUT" | grep "^Std Deviation:" | awk '{print $NF}')
PROG_LOG_VARIANCE=$(echo "$LOG_OUTPUT" | grep "^Variance:" | awk '{print $NF}')

printf "| %-12s | %15s | %15s | %-6s |\n" "Statistic" "bc Calculation" "Program Output" "Match"
printf "|--------------|-----------------|-----------------|--------|\n"
compare_values "Log Mean" "$LOG_MEAN" "$PROG_LOG_MEAN"
compare_values "Log StdDev" "$LOG_STDDEV" "$PROG_LOG_STDDEV"
compare_values "Log Variance" "$LOG_VARIANCE" "$PROG_LOG_VARIANCE"
echo ""

# --- Symmetry Verification (v1.12.0) ---
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
        echo "FAIL: pair ${SYM_SORTED[$i]} + ${SYM_SORTED[$((SYM_COUNT - 1 - i))]} = $PAIR_SUM, expected $(echo "2 * $SYM_CENTER" | bc -l)"
        PAIR_FAILURES=$((PAIR_FAILURES + 1))
    fi
done
if [[ $PAIR_FAILURES -eq 0 ]]; then
    printf "PASS: all %d sorted pairs sum to 2 * center (center=%.0f)\n" $((SYM_COUNT / 2)) "$SYM_CENTER"
else
    FAILURES=$((FAILURES + PAIR_FAILURES))
fi

TMPFILE5=$(mktemp)
echo "$SYMDATA" | tr ' ' '\n' > "$TMPFILE5"
SYM_OUTPUT=$(run_stats "$TMPFILE5")
rm "$TMPFILE5"

if echo "$SYM_OUTPUT" | grep -qF "Symmetric about 500 (20 pairs)"; then
    echo "PASS: program reports 'Symmetric about 500 (20 pairs)'"
else
    echo "FAIL: program did not report the expected symmetry line"
    FAILURES=$((FAILURES + 1))
fi
echo ""

# --- Skipped Lines Verification (v1.13.0) ---
echo "=============================================="
echo "Skipped Lines Verification"
echo "=============================================="
echo ""

# 2 invalid lines (abc, xyz); the 2 blank lines must not be counted
TMPFILE6=$(mktemp)
printf '10\n\nabc\n20\n\nxyz\n30\n' > "$TMPFILE6"
SKIP_OUTPUT=$(run_stats "$TMPFILE6" 2>/dev/null)
rm "$TMPFILE6"

SKIP_COUNT=$(echo "$SKIP_OUTPUT" | grep "^Skipped:" | awk '{print $2}' || true)
if [[ "$SKIP_COUNT" == "2" ]]; then
    echo "PASS: Skipped reports 2 invalid lines (blank lines not counted)"
else
    echo "FAIL: Skipped count is '$SKIP_COUNT', expected 2"
    FAILURES=$((FAILURES + 1))
fi
SKIP_DATA_COUNT=$(echo "$SKIP_OUTPUT" | grep "^Count:" | awk '{print $2}' || true)
if [[ "$SKIP_DATA_COUNT" == "3" ]]; then
    echo "PASS: Count is 3 valid values"
else
    echo "FAIL: Count is '$SKIP_DATA_COUNT', expected 3"
    FAILURES=$((FAILURES + 1))
fi
echo ""

# --- Input Order Verification (v1.13.0) ---
echo "=============================================="
echo "Input Order Verification (sorted input)"
echo "=============================================="
echo ""

TMPFILE7=$(mktemp)
echo "$DATA" | tr ' ' '\n' | sort -n > "$TMPFILE7"
ORDER_OUTPUT=$(run_stats "$TMPFILE7")
rm "$TMPFILE7"

ORDER_LINE=$(echo "$ORDER_OUTPUT" | grep "^Input Order:" || true)
if echo "$ORDER_LINE" | grep -q "ascending"; then
    echo "PASS: sorted input reports ascending"
else
    echo "FAIL: sorted input did not report ascending: $ORDER_LINE"
    FAILURES=$((FAILURES + 1))
fi
if echo "$ORDER_LINE" | grep -q "WARNING"; then
    echo "PASS: sorted input carries the sort-order warning"
else
    echo "FAIL: sorted input is missing the sort-order warning"
    FAILURES=$((FAILURES + 1))
fi
echo ""

# --- Trim Dataset (-T) Verification ---
echo "=============================================="
echo "Trim Dataset Verification (-T 10)"
echo "=============================================="
echo ""

TMPFILE4=$(mktemp)
echo "$DATA" | tr ' ' '\n' > "$TMPFILE4"

if [[ -f "./stats" ]]; then
    TRIMD_OUTPUT=$(./stats -T 10 "$TMPFILE4")
elif command -v go &> /dev/null; then
    TRIMD_OUTPUT=$(go run stats.go -T 10 "$TMPFILE4")
else
    echo "Error: Neither ./stats binary nor go command found"
    rm "$TMPFILE4"
    exit 1
fi

rm "$TMPFILE4"

echo "$TRIMD_OUTPUT"
echo ""

# Verify header line present with correct counts
TRIMD_HEADER=$(echo "$TRIMD_OUTPUT" | grep "^(trimmed dataset:")
if [[ -z "$TRIMD_HEADER" ]]; then
    echo "FAIL: Trim dataset header not found"
    FAILURES=$((FAILURES + 1))
else
    echo "PASS: Trim dataset header present: $TRIMD_HEADER"
    # Verify counts in header (31 → 25)
    if echo "$TRIMD_HEADER" | grep -q "31 → 25"; then
        echo "PASS: Header shows correct counts (31 → 25)"
    else
        echo "FAIL: Header does not show expected counts (31 → 25)"
        FAILURES=$((FAILURES + 1))
    fi
fi

# Verify Count shows 25
TRIMD_COUNT=$(echo "$TRIMD_OUTPUT" | grep "^Count:" | awk '{print $2}')
if [[ "$TRIMD_COUNT" == "25" ]]; then
    echo "PASS: Count is 25 (trimmed from 31)"
else
    echo "FAIL: Count is $TRIMD_COUNT, expected 25"
    FAILURES=$((FAILURES + 1))
fi

# Verify Trendline absent
TRIMD_TRENDLINE=$(echo "$TRIMD_OUTPUT" | grep "^Trendline:" || true)
if [[ -z "$TRIMD_TRENDLINE" ]]; then
    echo "PASS: Trendline absent (as expected for trimmed dataset)"
else
    echo "FAIL: Trendline should be absent for trimmed dataset"
    FAILURES=$((FAILURES + 1))
fi

# Verify footnote present
TRIMD_FOOTNOTE=$(echo "$TRIMD_OUTPUT" | grep "computed on trimmed dataset")
if [[ -n "$TRIMD_FOOTNOTE" ]]; then
    echo "PASS: Footnote present"
else
    echo "FAIL: Footnote not found"
    FAILURES=$((FAILURES + 1))
fi

# Verify MAD carries the trimmed-dataset star (v1.13.0)
if echo "$TRIMD_OUTPUT" | grep -q "^MAD\*:"; then
    echo "PASS: MAD is starred under -T"
else
    echo "FAIL: MAD should carry * under -T"
    FAILURES=$((FAILURES + 1))
fi

# Verify order-aware statistics are suppressed, like the Trendline (v1.13.0)
for LABEL in "Autocorrelation:" "Input Order:"; do
    if echo "$TRIMD_OUTPUT" | grep -q "^$LABEL"; then
        echo "FAIL: $LABEL should be absent under -T"
        FAILURES=$((FAILURES + 1))
    else
        echo "PASS: $LABEL absent under -T"
    fi
done

# Verify -e and -T are mutually exclusive (v1.13.0)
TMPFILE8=$(mktemp)
echo "$DATA" | tr ' ' '\n' > "$TMPFILE8"
set +e
EXCL_OUTPUT=$(run_stats -e 5 -T 10 "$TMPFILE8" 2>&1)
EXCL_STATUS=$?
set -e
rm "$TMPFILE8"
if [[ $EXCL_STATUS -ne 0 ]] && echo "$EXCL_OUTPUT" | grep -q "mutually exclusive"; then
    echo "PASS: -e and -T are mutually exclusive"
else
    echo "FAIL: expected -e/-T mutual exclusion error (exit=$EXCL_STATUS)"
    FAILURES=$((FAILURES + 1))
fi

echo ""

if [[ $FAILURES -eq 0 ]]; then
    echo "Verification complete. All values match."
    exit 0
else
    echo "Verification FAILED. $FAILURES value(s) did not match."
    exit 1
fi
