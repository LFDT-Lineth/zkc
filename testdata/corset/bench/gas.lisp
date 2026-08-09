(defconst
    EVM_INST_LT                               0x10
    WCP_INST_LEQ                              0x0F)

(module gas)

(defcolumns
  (INPUTS_AND_OUTPUTS_ARE_MEANINGFUL    :binary@prove)
  (FIRST                                :binary@prove)
  (CT                                   :i3)
  (CT_MAX                               :i3)
  (GAS_ACTUAL                           :i64)
  (GAS_COST                             :i64)
  (EXCEPTIONS_AHOY                      :binary@prove)
  (OUT_OF_GAS_EXCEPTION                 :binary@prove)
  (WCP_ARG1_LO                          :i128)
  (WCP_ARG2_LO                          :i128)
  (WCP_INST                             :byte@prove :display :opcode)
  (WCP_RES                              :binary@prove))

(defalias
  IOMF  INPUTS_AND_OUTPUTS_ARE_MEANINGFUL
  XAHOY EXCEPTIONS_AHOY
  OOGX  OUT_OF_GAS_EXCEPTION)


(module gas)

;;;;;;;;;;;;;;;;;;;;;;
;;                  ;;
;;  3.1 Binarities  ;;
;;                  ;;
;;;;;;;;;;;;;;;;;;;;;;
(defconstraint binary-constraints ()
  (if-not-zero OOGX
              (eq! XAHOY 1)))

;; others are done with binary@prove in columns.lisp

;;;;;;;;;;;;;;;;;;;;;;;;;
;;                     ;;
;;    3.2 Heartbeat    ;;
;;                     ;;
;;;;;;;;;;;;;;;;;;;;;;;;;
;; 1
(defconstraint first-row (:domain {0})
  (vanishes! IOMF))

;; 2
(defconstraint iomf-increments ()
  (or! (will-remain-constant! IOMF) (will-inc! IOMF 1)))

;; 3
(defconstraint iomf-vanishing-values-first ()
  (if-zero IOMF
           (vanishes! FIRST)))

(defconstraint iomf-vanishing-values-counter ()
  (if-zero IOMF
           (vanishes! (next CT))))

;; 4
(defconstraint instruction-counter-cycle-ct-max ()
  (if-not-zero IOMF
               (eq! CT_MAX
                    (- 2
                       (* XAHOY (- 1 OOGX))))))

(defconstraint instruction-counter-cycle-first ()
  (if-not-zero IOMF
               (if-zero CT
                        (eq! FIRST 1)
                        (eq! FIRST 0))))

(defconstraint instruction-counter-cycle-counter ()
  (if-not-zero IOMF
               (if-eq-else CT CT_MAX
                           (vanishes! (next CT))
                           (will-inc! CT 1))))

;; 5
(defconstraint final-row (:domain {-1})
  (if-not-zero IOMF
               (eq! CT CT_MAX)))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;;                             ;;
;;  3.3 Constancy constraints  ;;
;;                             ;;
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
(defconstraint counter-constancy-gas-actual ()
  (counter-constancy CT GAS_ACTUAL))

(defconstraint counter-constancy-gas-cost ()
  (counter-constancy CT GAS_COST))

(defconstraint counter-constancy-xahoy ()
  (counter-constancy CT XAHOY))

(defconstraint counter-constancy-oogx ()
  (counter-constancy CT OOGX))

;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;;                                     ;;
;;  3.4 Populating the lookup columns  ;;
;;                                     ;;
;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;;
;; NOTE: what follows amounts to a "call to LEQ" on the current row, i.e.
;; (0 <= GAS_ACTUAL) is TRUE.
(defconstraint asserting-the-leftover-gas-is-nonnegative-arg1 (:guard FIRST)
  (eq! WCP_ARG1_LO 0))

(defconstraint asserting-the-leftover-gas-is-nonnegative-arg2 (:guard FIRST)
  (eq! WCP_ARG2_LO GAS_ACTUAL))

(defconstraint asserting-the-leftover-gas-is-nonnegative-inst (:guard FIRST)
  (eq! WCP_INST WCP_INST_LEQ))

(defconstraint asserting-the-leftover-gas-is-nonnegative-res (:guard FIRST)
  (eq! WCP_RES 1))

;; as per the spec, this constraint the following
;; constraint is slightly useless ... not entirely,
;; though: it still asserts "smallness" so that it
;; should filter out MXPX induced out of gas exceptions.
;;
;; NOTE: what follows amounts to a "call to LEQ" on the next row, i.e.
;; (0 <= GAS_COST) is TRUE.
(defconstraint asserting-the-gas-cost-is-nonnegative-arg1 (:guard FIRST)
  (eq! (shift WCP_ARG1_LO 1) 0))

(defconstraint asserting-the-gas-cost-is-nonnegative-arg2 (:guard FIRST)
  (eq! (shift WCP_ARG2_LO 1) GAS_COST))

(defconstraint asserting-the-gas-cost-is-nonnegative-inst (:guard FIRST)
  (eq! (shift WCP_INST 1) WCP_INST_LEQ))

(defconstraint asserting-the-gas-cost-is-nonnegative-res (:guard FIRST)
  (eq! (shift WCP_RES 1) 1))

;; NOTE: what follows amounts to a "call to LT" two rows down, i.e.
;; (GAS_ACTUAL < GAS_COST) is OOGX (as predicted by the HUB).
(defconstraint asserting-either-sufficient-gas-or-insufficient-gas-arg1 (:guard FIRST)
  (if-zero (force-bin (* XAHOY (- 1 OOGX)))
           (eq! (shift WCP_ARG1_LO 2) GAS_ACTUAL)))

(defconstraint asserting-either-sufficient-gas-or-insufficient-gas-arg2 (:guard FIRST)
  (if-zero (force-bin (* XAHOY (- 1 OOGX)))
           (eq! (shift WCP_ARG2_LO 2) GAS_COST)))

(defconstraint asserting-either-sufficient-gas-or-insufficient-gas-inst (:guard FIRST)
  (if-zero (force-bin (* XAHOY (- 1 OOGX)))
           (eq! (shift WCP_INST 2) EVM_INST_LT)))

(defconstraint asserting-either-sufficient-gas-or-insufficient-gas-res (:guard FIRST)
  (if-zero (force-bin (* XAHOY (- 1 OOGX)))
           (eq! (shift WCP_RES 2) OOGX)))
