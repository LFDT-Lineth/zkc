;; As lookup_27, but without a target selector.
(defcolumns (X :i17) (P :binary) (Y :i16))
;; use of selector
(deflookup (test :unchecked) ((P X)) ((_ Y)))
