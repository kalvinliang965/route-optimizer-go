# Project README: Manhattan Multi-Stop Route Optimizer

## Overview
A high-performance, command-line routing engine written in **Go**. The tool computes optimal multi-stop travel paths (limited to a depot plus up to 14 localized stops) across Manhattan using brute-force permutations backed by a **Bounded Min-Heap** algorithm (`container/heap`) to extract the top-$K$ shortest routes instantly.

---

## What We Currently Have (Phase 1: Core Engine & CLI)

* **Permutation Engine:** Exhaustively calculates valid route combinations starting from a fixed depot index (`0`).
* **Bounded Top-$K$ Min-Heap (`container/heap`):** Efficiently caps memory consumption and stores the top performing paths sorted by duration.
* **Interactive Terminal CLI Loop (`bufio`):** A REPL-style menu system allowing users to view stops, execute calculations, and inspect ranked results cleanly in the terminal.
* **Structured Output:** Displays formatted options complete with rank, total travel duration, and step-by-step stop listings.

---

## Next Steps (Phase 2: Local Architecture & Usability)

* **Configuration & Input Parsing:** Support file inputs (`JSON`/`CSV`) for importing custom stop schedules and street locations dynamically instead of hardcoded structs.
* **Distance Matrix Integration:** Implement structural handlers to easily swap between Euclidean distance estimations, mock Manhattan grid layouts, or pre-calculated static distance tables.
* **Error Resilience:** Add graceful edge-case handling for malformed routes, missing stop mappings, and bounds checking.

---

## Future Final Goal (Phase 3: Real-Time Manhattan Traffic Data)

The ultimate vision is to evolve this static calculator into a **zero-cost, real-time context-aware engine** leveraging free public municipal infrastructure data sources within Manhattan.

### The Target Architecture:
1. **Free Data Providers:** 
   * Integrating **NYC Open Data (DOT Traffic Speeds)** or the **511NY REST API** (using background caching workers to stay well beneath strict rate limits like 10 calls/minute).
2. **The Caching & Matching Pipeline:**
   * A background worker routine in Go that periodically fetches live link speeds and incident alerts.
   * A spatial coordinate mapping layer that correlates raw NYC segment IDs/sensors to local route matrices.
3. **Dynamic Edge-Weight Updates:**
   * Injecting real-time congestion multipliers directly into the `matrix [][]float64` weights *before* passing the data to the permutation solver and min-heap.