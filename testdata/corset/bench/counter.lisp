;; ===================================================
;; Constraints
;; ===================================================
(defcolumns (STAMP :i16) (CT :i4))

;; In the first row, STAMP is always zero.  This allows for an
;; arbitrary amount of padding at the beginning which has no function.
(defconstraint first (:domain {0}) (== STAMP 0))

;; In the last row of a valid frame, the counter must have its max
;; value.  This ensures that all non-padding frames are complete.
(defconstraint last (:domain {-1} :guard STAMP) (== CT 3))

;; STAMP either remains constant, or increments by one.
(defconstraint increment () (∨
                      ;; STAMP[k] == STAMP[k+1]
                      (== (shift STAMP 1) STAMP)
                      ;; Or, STAMP[k]+1 == STAMP[k+1]
                      (== (shift STAMP 1) (+ STAMP 1))))

;; If STAMP changes, counter resets to zero.
(defconstraint reset () (∨
                  ;; STAMP[k] == STAMP[k+1]
                  (== (shift STAMP 1) STAMP)
                  ;; Or, CT[k+1] == 0
                  (== (shift CT 1) 0)))

;; If STAMP non-zero and reaches end-of-cycle, then stamp increments;
;; otherwise, counter increments.
(defconstraint heartbeat (:guard STAMP)
  ;; If CT == 3
  (if (== CT 3)
      ;; Then, STAMP[k]+1 == STAMP[k]
      (== (shift STAMP 1) (+ STAMP 1))
      ;; Else, CT[k]+1 == CT[k]
      (== (shift CT 1) (+ CT 1))))
