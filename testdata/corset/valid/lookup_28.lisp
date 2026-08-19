(defcolumns (X :i16) (Y :i16) (P :i16) (SEL :binary))
;; SEL holds the value of the expression (- P (prev P))
(defconstraint sel_def () (== SEL (- P (shift P -1))))
;; example use of selector
(defclookup l1 (Y) SEL (X))

;; enforce (P - (prev P)) is binary.
(defconstraint inc ()
  (∨
   (== P (shift P -1))
   (== (- P 1) (shift P -1))))
