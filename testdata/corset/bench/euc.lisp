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
  (vanishes! IOMF))

;; In padding rows, nothing is done and the counter stays at zero.
(defconstraint heartbeat-padding-done ()
  (if-zero IOMF
           (vanishes! DONE)))

(defconstraint heartbeat-padding-counter ()
  (if-zero IOMF
           (vanishes! (next CT))))

;; Outside of padding, the module remains active.
(defconstraint heartbeat-iomf-latches ()
  (if-not-zero IOMF
               (eq! (next IOMF) 1)))

;; On the final row of a frame, the division is done and the counter is reset.
(defconstraint heartbeat-final-row-done ()
  (if-not-zero IOMF
               (if-zero (- CT_MAX CT)
                        (eq! DONE 1))))

(defconstraint heartbeat-final-row-counter ()
  (if-not-zero IOMF
               (if-zero (- CT_MAX CT)
                        (vanishes! (next CT)))))

;; On rows within a frame, the division is not done and the counter increases.
(defconstraint heartbeat-inner-row-done ()
  (if-not-zero IOMF
               (if-not-zero (- CT_MAX CT)
                            (vanishes! DONE))))

(defconstraint heartbeat-inner-row-counter ()
  (if-not-zero IOMF
               (if-not-zero (- CT_MAX CT)
                            (will-inc! CT 1))))

(defconstraint ctmax ()
  (eq! (~ (- CT MAX_INPUT_LENGTH))
       1))

(defconstraint counter-constancies ()
  (counter-constancy CT CT_MAX))

(defconstraint byte-decomposition-divisor ()
  (byte-decomposition CT DIVISOR DIVISOR_BYTE))

(defconstraint byte-decomposition-quotient ()
  (byte-decomposition CT QUOTIENT QUOTIENT_BYTE))

(defconstraint byte-decomposition-remainder ()
  (byte-decomposition CT REMAINDER REMAINDER_BYTE))

(defconstraint result-euclidean-division (:guard DONE)
  (eq! DIVIDEND
       (+ (* DIVISOR QUOTIENT) REMAINDER)))

(defconstraint result-ceiling (:guard DONE)
  (if-zero (* DIVIDEND REMAINDER)
           (eq! CEIL QUOTIENT)
           (eq! CEIL (+ QUOTIENT 1))))
