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

;; Constructs two nibbles into a byte
(defpurefun (as_u8 b1 b0) (+ (* 16 b1) b0))

;; Byte decompositions
(defconstraint decompositions ()
  (begin
   ;; X
   (eq! X (as_u8 [XS 2] [XS 1]))
   ;; Y
   (eq! Y (as_u8 [YS 2] [YS 1]))))

;; Constraint on lower half
(defconstraint low4 (:guard ST)
  (vanishes! (+ (* 16 CARRY) (- [XS 1] [YS 1] 1))))

;; Constraint on upper half
(defconstraint high4 (:guard ST)
  (vanishes! (- [XS 2] [YS 2] CARRY)))
