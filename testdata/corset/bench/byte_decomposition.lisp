(defcolumns
  (ST :i32)
  (CT :i4)
  (BYTE :i8@prove)
  (ARG :i32))

;; In the first row, ST is always zero.  This allows for an
;; arbitrary amount of padding at the beginning which has no function.
(defconstraint first (:domain {0}) (== ST 0))

;; In the last row of a valid frame, the counter must have its max
;; value.  This ensures that all non-padding frames are complete.
(defconstraint last (:domain {-1} :guard ST)
  ;; CT[$] == 3
  (== CT 3))

;; ST either remains constant, or increments by one.
(defconstraint increment ()
  (∨
   ;; ST[k] == ST[k+1]
   (== ST (shift ST 1))
   ;; Or, ST[k]+1 == ST[k+1]
   (== (+ 1 ST) (shift ST 1))))

;; If ST changes, counter resets to zero.
(defconstraint reset ()
  (∨
   ;; ST[k] == ST[k+1]
   (== ST (shift ST 1))
   ;; Or, CT[k+1] == 0
   (== (shift CT 1) 0)))

;; Increment or reset counter
(defconstraint heartbeat (:guard ST)
  ;; If CT[k] == 3
  (if (== CT 3)
              ;; Then, ST[k]+1 = ST[k+1]
              (== (shift ST 1) (+ 1 ST))
              ;; Else, CT[k]+1 == CT[k+1]
              (== (+ 1 CT) (shift CT 1))))

;; Argument accumulates byte values.
(defconstraint accumulator (:guard ST)
  ;; If CT[k] == 0
  (if (== CT 0)
              ;; Then, ARG == BYTE
              (== ARG BYTE)
              ;; Else, ARG = BYTE[k] + 256*BYTE[k-1]
              (== ARG (+ BYTE (* 256 (shift ARG -1))))))
