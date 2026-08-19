;; Shift & Add Multiplier
;;
;;     |x|y|
;;     |a|b|
;; ---------
;; | | |*|*| +b.y
;; | |*|*| | +b.x
;; | |*|*| | +a.y
;; |*|*| | | +a.x
(defcolumns
  (ST :i32)
  (CT :i4)
  ;; nibbles
  (ARG1 :i4@prove :array [0:1])
  (ARG2 :i4@prove :array [0:1])
  (RES :i4@prove :array [0:3]))

;; NOTE: this is not really a finished example, since it doesn't
;; actually use carry lines (yet).  These would be needed if small
;; fields are used to prevent overflow.

;; ===================================================================
;; Control
;; ===================================================================

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
  (if
   ;; ST[k] == ST[k+1]
   (== ST (shift ST 1))
   (== 0 0)
   ;; Or, ST[k]+1 == ST[k+1]
   (== (+ 1 ST) (shift ST 1))))

;; If ST changes, counter resets to zero.
(defconstraint reset ()
  (if
   ;; ST[k] == ST[k+1]
   (== ST (shift ST 1))
   (== 0 0)
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

;; ===================================================================
;; Multipilier
;; ===================================================================

;; RES() == (+ (* 4096 [RES 3]) (* 256 [RES 2]) (* 16 [RES 1]) [RES 0])

(defconstraint line_1 (:guard ST)
  (if (== CT 0)
      (== (+ (* 4096 [RES 3]) (* 256 [RES 2]) (* 16 [RES 1]) [RES 0])
          (* [ARG1 0] [ARG2 0]))))

(defconstraint line_2 (:guard ST)
  (if (== CT 1)
      (== (+ (* 4096 [RES 3]) (* 256 [RES 2]) (* 16 [RES 1]) [RES 0])
          (+ (shift (+ (* 4096 [RES 3]) (* 256 [RES 2]) (* 16 [RES 1]) [RES 0]) -1)
             (* 16 [ARG1 0] [ARG2 1])))))

(defconstraint line_3 (:guard ST)
  (if (== CT 2)
      (== (+ (* 4096 [RES 3]) (* 256 [RES 2]) (* 16 [RES 1]) [RES 0])
          (+ (shift (+ (* 4096 [RES 3]) (* 256 [RES 2]) (* 16 [RES 1]) [RES 0]) -1)
             (* 16 [ARG1 1] [ARG2 0])))))

(defconstraint line_4 (:guard ST)
  (if (== CT 3)
      (== (+ (* 4096 [RES 3]) (* 256 [RES 2]) (* 16 [RES 1]) [RES 0])
          (+ (shift (+ (* 4096 [RES 3]) (* 256 [RES 2]) (* 16 [RES 1]) [RES 0]) -1)
             (* 256 [ARG1 1] [ARG2 1])))))
