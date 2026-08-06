(defcolumns (X :i16) (Y :i16) (P :i16) (SEL :binary))
(defpurefun (prev (X :any)) (shift X -1))
;; define selector function
(defun ((selector :binary :force)) (- P (prev P)))
;; SEL holds the value of the expression (selector)
(defconstraint sel_def () (== SEL (selector)))
;; example use of selector
(defclookup l1 (Y) SEL (X))

;; enforce (P - (prev P)) is binary.
(defconstraint inc ()
  (∨
   (== P (prev P))
   (== (- P 1) (prev P))))
