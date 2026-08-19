(module euc)

(defcolumns
  (IOMF :binary@prove)
  (CT :i8)
  (CT_MAX :i8)
  (DIVIDEND :i64)
  (DIVISOR :i64)
  (QUOTIENT :i64)
  (REMAINDER :i64)
  (CEIL :i64)
  (DONE :binary)
  (DIVISOR_BYTE :byte@prove)
  (QUOTIENT_BYTE :byte@prove)
  (REMAINDER_BYTE :byte@prove))

(defconst
  MMEDIUM                                   8)

(module euc)

(defconst
  MAX_INPUT_LENGTH MMEDIUM)

(defconstraint first-row (:domain {0})
  (== IOMF 0))

;; In padding rows, nothing is done and the counter stays at zero.
(defconstraint heartbeat-padding-done ()
  (if (== IOMF 0)
      (== DONE 0)))

(defconstraint heartbeat-padding-counter ()
  (if (== IOMF 0)
      (== (shift CT 1) 0)))

;; Outside of padding, the module remains active.
(defconstraint heartbeat-iomf-latches ()
  (if (!= IOMF 0)
      (== (shift IOMF 1) 1)))

;; On the final row of a frame, the division is done and the counter is reset.
(defconstraint heartbeat-final-row-done ()
  (if (!= IOMF 0)
      (if (== (- CT_MAX CT) 0)
          (== DONE 1))))

(defconstraint heartbeat-final-row-counter ()
  (if (!= IOMF 0)
      (if (== (- CT_MAX CT) 0)
          (== (shift CT 1) 0))))

;; On rows within a frame, the division is not done and the counter increases.
(defconstraint heartbeat-inner-row-done ()
  (if (!= IOMF 0)
      (if (!= (- CT_MAX CT) 0)
          (== DONE 0))))

(defconstraint heartbeat-inner-row-counter ()
  (if (!= IOMF 0)
      (if (!= (- CT_MAX CT) 0)
          (== (shift CT 1) (+ CT 1)))))

(defconstraint ctmax ()
  (!= CT MAX_INPUT_LENGTH))

(defconstraint counter-constancies ()
  (if (!= CT 0) (== CT_MAX (shift CT_MAX -1))))

(defconstraint byte-decomposition-divisor ()
  (if (== CT 0) (== DIVISOR DIVISOR_BYTE) (== DIVISOR (+ (* 256 (shift DIVISOR -1)) DIVISOR_BYTE))))

(defconstraint byte-decomposition-quotient ()
  (if (== CT 0) (== QUOTIENT QUOTIENT_BYTE) (== QUOTIENT (+ (* 256 (shift QUOTIENT -1)) QUOTIENT_BYTE))))

(defconstraint byte-decomposition-remainder ()
  (if (== CT 0) (== REMAINDER REMAINDER_BYTE) (== REMAINDER (+ (* 256 (shift REMAINDER -1)) REMAINDER_BYTE))))

(defconstraint result-euclidean-division (:guard DONE)
  (== DIVIDEND
      (+ (* DIVISOR QUOTIENT) REMAINDER)))

(defconstraint result-ceiling (:guard DONE)
  (if (== (* DIVIDEND REMAINDER) 0)
      (== CEIL QUOTIENT)
      (== CEIL (+ QUOTIENT 1))))
