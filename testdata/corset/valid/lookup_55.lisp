;; As lookup_28, but without a target selector.
(defcolumns (X :i16) (Y :i16) (P :i16) (SEL :binary))
;; SEL holds the value of the expression (- P (prev P))
(defconstraint sel_def () (== SEL (- P (shift P -1))))
;; example use of selector
(deflookup l1 ((SEL X)) ((_ Y)))

;; enforce (P - (prev P)) is binary.
(defconstraint inc ()
  (∨
   (== P (shift P -1))
   (== (- P 1) (shift P -1))))
