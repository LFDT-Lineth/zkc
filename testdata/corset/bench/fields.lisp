(defcolumns
    (ST :binary@prove)
    ;; Words
    (X :i8@prove)
    (Y :i8@prove)
    ;; Bytes
    (XS :i4@prove :array [2])
    (YS :i4@prove :array [2])
    ;; Carry flag
    (CARRY :binary@prove))

;; Byte decompositions
(defconstraint decompositions-x ()
   (== X (+ (* 16 [XS 2]) [XS 1])))

(defconstraint decompositions-y ()
   (== Y (+ (* 16 [YS 2]) [YS 1])))

;; Constraint on lower half
(defconstraint low4 (:guard ST)
  (== (+ (* 16 CARRY) (- [XS 1] [YS 1] 1)) 0))

;; Constraint on upper half
(defconstraint high4 (:guard ST)
  (== (- [XS 2] [YS 2] CARRY) 0))
