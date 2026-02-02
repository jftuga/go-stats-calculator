#!/bin/bash
# verify_stats.sh - Independent verification of stats calculations using bc
# Uses the same 31-number dataset from stats_test.go
# Created on MacOS Sequoia 15.7.3
# COMPATIBILITY: Must work with bash 5.2+ (GitHub Actions requirement)

set -e

# Dataset (same as testData in stats_test.go)
DATA="5 10 15.5 20 25 30 35 40 45 50 55 60 65 70 75.25 80 85 90 95 100 12.5 37.5 62.5 87.5 50 50 50 3 150 7.75 42"
COUNT=31

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

if [[ $FAILURES -eq 0 ]]; then
    echo "Verification complete. All values match."
    exit 0
else
    echo "Verification FAILED. $FAILURES value(s) did not match."
    exit 1
fi
