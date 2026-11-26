(defcolumns (X :i17) (P :binary) (Y :i16) (one :i1 :constant 1))
;; use of selector
(deflookup (test :unchecked) ((P X)) ((one Y)))
