(defcolumns
    (ST :binary@prove)
    ;; Words
    (X :i8@prove)
    (Y :i8@prove)
    ;; Bytes
    (XS_1 :i4@prove)
    (XS_2 :i4@prove)
    (YS_1 :i4@prove)
    (YS_2 :i4@prove)
    ;; Carry flag
    (CARRY :binary@prove))

;; Byte decompositions
(defconstraint decompositions-x ()
   (== X (+ (* 16 XS_2) XS_1)))

(defconstraint decompositions-y ()
   (== Y (+ (* 16 YS_2) YS_1)))

;; Constraint on lower half
(defconstraint low4 (:guard ST)
  (== (+ (* 16 CARRY) (- XS_1 YS_1 1)) 0))

;; Constraint on upper half
(defconstraint high4 (:guard ST)
  (== (- XS_2 YS_2 CARRY) 0))
