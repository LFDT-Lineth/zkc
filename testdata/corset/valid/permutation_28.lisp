;; Permutation whose source selector is defined in terms of another column.
(defcolumns (X :i16) (Y :i16) (P :i16) (SEL :binary) (one :i1 :constant 1))
;; SEL holds the value of the expression (- P (prev P)).
(defconstraint sel_def () (== SEL (- P (shift P -1))))
;; SEL selects the source rows.
(defpermutation l1 ((SEL X)) ((one Y)))

;; Enforce that (- P (prev P)) is binary.
(defconstraint inc ()
  (∨
   (== P (shift P -1))
   (== (- P 1) (shift P -1))))
